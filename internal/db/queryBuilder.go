package db

import (
	"fmt"
	"strings"

	"github.com/gadget-inc/dateilager/internal/pb"
)

type queryBuilder struct {
	project                int64
	vrange                 VersionRange
	objectQuery            *pb.ObjectQuery
	availableCacheVersions []int64
	withHash               bool
}

func newQueryBuilder(project int64, vrange VersionRange, objectQuery *pb.ObjectQuery, availableCacheVersions []int64, withHash bool) queryBuilder {
	return queryBuilder{
		project:                project,
		vrange:                 vrange,
		objectQuery:            objectQuery,
		availableCacheVersions: availableCacheVersions,
		withHash:               withHash,
	}
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
		SELECT o.path, o.mode, o.size, h.hash IS NOT NULL as is_cached, %s, o.packed, false AS deleted, %s
		FROM possible_objects o
		LEFT JOIN dl.contents c
		  ON o.hash = c.hash
		 %s
	    LEFT JOIN cached_object_hashes h
		  ON h.hash = o.hash
		WHERE o.project = __project__
		  AND o.start_version > __start_version__
		  AND o.start_version <= __stop_version__
		  AND (o.stop_version IS NULL OR o.stop_version > __stop_version__)
		  %s
		  %s
		ORDER BY o.path
	`

	bytesSelector := "CASE WHEN h.hash IS NULL THEN c.bytes ELSE NULL END as bytes"
	contentsPredicate := ""
	if !qb.objectQuery.WithContent {
		bytesSelector = "c.names_tar as bytes"
		contentsPredicate = "AND o.packed IS true"
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

	hashSelector := "null::hash as hash"
	if qb.withHash {
		hashSelector = "o.hash"
	}

	return fmt.Sprintf(template, bytesSelector, hashSelector, contentsPredicate, pathPredicate, ignoresPredicate)
}

func (qb *queryBuilder) removedObjectsCTE() string {
	template := `
		SELECT o.path, o.mode, 0 AS size, ''::bytea as bytes, o.packed, true AS deleted, null::hash as hash
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
		SELECT DISTINCT(UNNEST(hashes)) as hash FROM dl.cache_versions WHERE version = ANY (__available_cache_versions__)
	`
}

func (qb *queryBuilder) queryWithoutRemovals() string {
	template := `
		WITH possible_objects AS (
		%s
		), cached_object_hashes AS (
		%s
		), updated_objects AS (
		%s
		)
		%s
	`

	selectStatement := `
		SELECT path, mode, size, is_cached, bytes, packed, deleted, (hash).h1, (hash).h2
		FROM updated_objects
	`

	return fmt.Sprintf(template, qb.possibleObjectsCTE(false), qb.cachedObjectHashesCTE(), qb.updatedObjectsCTE(), selectStatement)
}

func (qb *queryBuilder) queryWithRemovals() string {
	template := `
		WITH possible_objects AS (
		%s
		), cached_object_hashes AS (
		%s
		), updated_objects AS (
		%s
		), removed_objects AS (
		%s
		)
		%s
	`

	selectStatement := `
		SELECT path, mode, size, is_cached, bytes, packed, deleted, (hash).h1, (hash).h2
		FROM updated_objects
		UNION ALL
		SELECT path, mode, size, false, bytes, packed, deleted, (hash).h1, (hash).h2
		FROM removed_objects
	`
	return fmt.Sprintf(template, qb.possibleObjectsCTE(true), qb.cachedObjectHashesCTE(), qb.updatedObjectsCTE(), qb.removedObjectsCTE(), selectStatement)
}

func (qb *queryBuilder) replaceQueryArgs(query string) (string, []any) {
	argNames := []string{"__project__", "__start_version__", "__stop_version__", "__available_cache_versions__"}
	args := []any{qb.project, qb.vrange.From, qb.vrange.To, qb.availableCacheVersions}

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

	for idx, name := range argNames {
		query = strings.ReplaceAll(query, name, fmt.Sprintf("$%d", idx+1))
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
