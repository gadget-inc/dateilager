package web

import (
	"context"
	"fmt"
	"net/http"
	"text/template"
	"time"

	"github.com/gadget-inc/dateilager/internal/pb"
	"github.com/gadget-inc/dateilager/pkg/server"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v4"
	"go.uber.org/zap"
)

type ChiMiddleware = func(next http.Handler) http.Handler

func logger(log *zap.Logger) ChiMiddleware {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			t1 := time.Now()
			defer func() {
				log.Info("Request",
					zap.String("proto", r.Proto),
					zap.String("path", r.URL.Path),
					zap.Duration("lat", time.Since(t1)),
					zap.Int("status", ww.Status()),
					zap.Int("size", ww.BytesWritten()),
					zap.String("reqId", middleware.GetReqID(r.Context())))
			}()

			next.ServeHTTP(ww, r)
		}
		return http.HandlerFunc(fn)
	}
}

type Project struct {
	Id            int64
	LatestVersion int64
}

func (p *Project) LinkLatestVersion() string {
	return fmt.Sprintf("/projects/%d/versions/%d", p.Id, p.LatestVersion)
}

func (p *Project) LinkVersion(version int64) string {
	return fmt.Sprintf("/projects/%d/versions/%d", p.Id, version)
}

type IndexData struct {
	Projects []Project
}

func fetchIndexData(ctx context.Context, tx pgx.Tx) (*IndexData, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, latest_version
		FROM dl.projects
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("fetch index data: %w", err)
	}

	data := IndexData{}

	for rows.Next() {
		var id, version int64

		err = rows.Scan(&id, &version)
		if err != nil {
			return nil, fmt.Errorf("fetch index data scan: %w", err)
		}

		data.Projects = append(data.Projects, Project{
			Id: id, LatestVersion: version,
		})
	}

	return &data, nil
}

type VersionData struct {
	Project int64
	Version int64
	Objects []*pb.Object
}

func fetchVersionData(ctx context.Context, tx pgx.Tx) (*VersionData, error) {

}

func NewWebServer(log *zap.Logger, pool *server.DbPoolConnector) *chi.Mux {
	router := chi.NewRouter()

	router.Use(middleware.RequestID)
	router.Use(logger(log))

	indexTmpl := template.Must(template.ParseFiles("pkg/web/templates/index.html.tmpl"))

	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		tx, close, err := pool.Connect(r.Context())
		if err != nil {
			log.Error("cannot connect to pool")
			http.Error(w, http.StatusText(500), 500)
			return
		}
		defer close()

		data, err := fetchIndexData(r.Context(), tx)
		if err != nil {
			log.Error("fetch index data", zap.Error(err))
			http.Error(w, http.StatusText(500), 500)
			return
		}

		indexTmpl.Execute(w, data)
	})

	router.Route("/projects/{projectId}/versions", func(router chi.Router) {

		router.Get("/{version}", func(w http.ResponseWriter, r *http.Request) {
			projectId := chi.URLParam(r, "projectId")
			version := chi.URLParam(r, "version")

		})
	})

	return router
}
