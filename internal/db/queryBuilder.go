package db

import (
	"fmt"
	"strings"

	"github.com/gadget-inc/dateilager/internal/pb"
)

type queryBuilder struct {
	project       int64
	vrange        VersionRange
	objectQuery   *pb.ObjectQuery
	cacheVersions []int64
	argsOffset    int
}

func newQueryBuilder(project int64, vrange VersionRange, objectQuery *pb.ObjectQuery) *queryBuilder {
	return &queryBuilder{
		project:       project,
		vrange:        vrange,
		objectQuery:   objectQuery,
		cacheVersions: nil,
		argsOffset:    0,
	}
}

func (qb *queryBuilder) withCacheVersions(cacheVersions []int64) *queryBuilder {
	qb.cacheVersions = cacheVersions
	return qb
}

func (qb *queryBuilder) withArgsOffset(offset int) *queryBuilder {
	qb.argsOffset = offset
	return qb
}

func (qb *queryBuilder) possibleObjectsCTE(withRemovals bool) string {
	if withRemovals {
		return `
			SELECT *
			FROM dl.objects
			WHERE project = __project__
			  AND (
				-- live objects
				(
				  start_version > __start_version__
				  AND start_version <= __stop_version__
				  AND (stop_version IS NULL OR stop_version > __stop_version__)
				)
				OR
				-- removed objects
				(
				  start_version <= __stop_version__
				  AND stop_version > __start_version__
				  AND stop_version <= __stop_version__
				)
			  )
		`
	} else {
		return `
			SELECT *
			FROM dl.objects
			WHERE project = __project__
			  AND start_version > __start_version__
			  AND start_version <= __stop_version__
			  AND (stop_version IS NULL OR stop_version > __stop_version__)
		`
	}
}

func (qb *queryBuilder) updatedObjectsCTE() string {
	template := `
			SELECT o.path, o.mode, o.size, %s, o.packed, false AS deleted, o.hash
			FROM possible_objects o
			%s
			WHERE o.project = __project__
			AND o.start_version > __start_version__
			AND o.start_version <= __stop_version__
			AND (o.stop_version IS NULL OR o.stop_version > __stop_version__)
			%s
			%s
			ORDER BY o.path
	`

	isCachedSelector := "false AS is_cached"
	cacheJoin := ""
	if len(qb.cacheVersions) > 0 {
		isCachedSelector = "h.hash IS NOT NULL AS is_cached"
		cacheJoin = `LEFT JOIN cached_object_hashes h
			       ON h.hash = o.hash`
	}

	pathPredicate := ""
	if qb.objectQuery.Path != "" {
		if qb.objectQuery.IsPrefix {
			pathPredicate = "AND o.path LIKE __path__"
		} else {
			pathPredicate = "AND o.path = __path__"
		}
	}

	ignoresPredicate := ""
	if len(qb.objectQuery.Ignores) > 0 {
		ignoresPredicate = "AND o.path NOT LIKE ALL(__ignores__::text[])"
	}

	return fmt.Sprintf(template, isCachedSelector, cacheJoin, pathPredicate, ignoresPredicate)
}

func (qb *queryBuilder) removedObjectsCTE() string {
	template := `
			SELECT o.path, o.mode, 0 AS size, o.packed, true AS deleted, '(00000000-0000-0000-0000-000000000000,00000000-0000-0000-0000-000000000000)'::hash AS hash
			FROM possible_objects o
			WHERE o.project = __project__
			AND o.start_version <= __start_version__
			AND o.stop_version > __start_version__
			AND o.stop_version <= __stop_version__
			%s
			AND (
			    -- Skip removing files if they are in the updated_objects list
			    (RIGHT(o.path, 1) != '/' AND o.path NOT IN (SELECT path FROM updated_objects))
			    OR
			    -- Skip removing empty directories if any updated_objects are within that directory
			    (RIGHT(o.path, 1) = '/' AND NOT EXISTS (SELECT true FROM updated_objects WHERE STARTS_WITH(path, o.path)))
			)
			ORDER BY o.path
	`

	ignoresPredicate := ""
	if len(qb.objectQuery.Ignores) > 0 {
		ignoresPredicate = "AND o.path NOT LIKE ALL(__ignores__::text[])"
	}

	return fmt.Sprintf(template, ignoresPredicate)
}

func (qb *queryBuilder) cachedObjectHashesCTE() string {
	return `
			SELECT DISTINCT(UNNEST(hashes)) AS hash
			FROM dl.cache_versions
			WHERE version = ANY(__cache_versions__)
	`
}

func (qb *queryBuilder) queryWithoutRemovals() string {
	template := `
		WITH possible_objects AS (
		%s
		), %supdated_objects AS (
		%s
		)
		%s
	`

	cacheCte := ""
	if len(qb.cacheVersions) > 0 {
		cacheCte = fmt.Sprintf(`cached_object_hashes AS (
		%s
		), `, qb.cachedObjectHashesCTE())
	}

	selectStatement := `
		SELECT path, mode, size, is_cached, packed, deleted, (hash).h1, (hash).h2
		FROM updated_objects
	`

	return fmt.Sprintf(template, qb.possibleObjectsCTE(false), cacheCte, qb.updatedObjectsCTE(), selectStatement)
}

func (qb *queryBuilder) queryWithRemovals() string {
	template := `
		WITH possible_objects AS (
		%s
		), %supdated_objects AS (
		%s
		), removed_objects AS (
		%s
		)
		%s
	`

	cacheCte := ""
	if len(qb.cacheVersions) > 0 {
		cacheCte = fmt.Sprintf(`cached_object_hashes AS (
		%s
		), `, qb.cachedObjectHashesCTE())
	}

	selectStatement := `
		SELECT path, mode, size, is_cached, packed, deleted, (hash).h1, (hash).h2
		FROM updated_objects
		UNION ALL
		SELECT path, mode, size, false, packed, deleted, (hash).h1, (hash).h2
		FROM removed_objects
	`
	return fmt.Sprintf(template, qb.possibleObjectsCTE(true), cacheCte, qb.updatedObjectsCTE(), qb.removedObjectsCTE(), selectStatement)
}

func (qb *queryBuilder) replaceQueryArgs(query string) (string, []any) {
	argNames := []string{"__project__", "__start_version__", "__stop_version__"}
	args := []any{qb.project, qb.vrange.From, qb.vrange.To}

	if qb.objectQuery.Path != "" {
		argNames = append(argNames, "__path__")

		if qb.objectQuery.IsPrefix {
			args = append(args, fmt.Sprintf("%s%%", qb.objectQuery.Path))
		} else {
			args = append(args, qb.objectQuery.Path)
		}
	}

	if len(qb.objectQuery.Ignores) > 0 {
		argNames = append(argNames, "__ignores__")

		ignorePatterns := []string{}
		for _, ignore := range qb.objectQuery.Ignores {
			ignorePatterns = append(ignorePatterns, fmt.Sprintf("%s%%", ignore))
		}

		args = append(args, ignorePatterns)
	}

	if len(qb.cacheVersions) > 0 {
		argNames = append(argNames, "__cache_versions__")
		args = append(args, qb.cacheVersions)
	}

	for idx, name := range argNames {
		query = strings.ReplaceAll(query, name, fmt.Sprintf("$%d", qb.argsOffset+idx+1))
	}

	return query, args
}

func (qb *queryBuilder) build() (string, []any) {
	var query string

	if qb.vrange.From == 0 {
		query = qb.queryWithoutRemovals()
	} else {
		query = qb.queryWithRemovals()
	}

	query, args := qb.replaceQueryArgs(query)

	return query, args
}
