package web

import (
	"context"
	"fmt"
	"html"
	"io/fs"
	"net/http"
	"path/filepath"
	"strconv"
	"text/template"
	"time"

	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

func error500(log *zap.Logger, w http.ResponseWriter, message string, err error, fields ...zap.Field) {
	fields = append(fields, zap.Error(err))

	log.Error(message, fields...)
	http.Error(w, http.StatusText(500), 500)
}

func error400(log *zap.Logger, w http.ResponseWriter, message string, err error, fields ...zap.Field) {
	fields = append(fields, zap.Error(err))

	log.Error(message, fields...)
	http.Error(w, http.StatusText(400), 400)
}

func logger(log *zap.Logger) func(next http.Handler) http.Handler {
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

func NewWebServer(log *zap.Logger, dlc *client.Client, assetsDir string) (*chi.Mux, error) {
	router := chi.NewRouter()

	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(logger(log))
	router.Use(middleware.Recoverer)

	tmpls, err := template.ParseGlob(filepath.Join(assetsDir, "templates", "*.tmpl"))
	if err != nil {
		return nil, fmt.Errorf("parsing templates from %v: %w", filepath.Join(assetsDir, "templates"), err)
	}

	router.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(assetsDir, "static", "favicon.ico"))
	})

	router.Get("/", getIndex(log, dlc, tmpls))

	router.Route("/projects/{projectId}/versions", func(router chi.Router) {
		router.Get("/{version}", getVersion(log, dlc, tmpls))
	})

	return router, nil
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

func fetchIndexData(ctx context.Context, dlc *client.Client) (*IndexData, error) {
	projects, err := dlc.ListProjects(ctx)
	if err != nil {
		return nil, err
	}

	data := IndexData{}

	for _, project := range projects {
		data.Projects = append(data.Projects, Project{
			Id:            project.Id,
			LatestVersion: project.Version,
		})
	}

	return &data, nil
}

func getIndex(log *zap.Logger, dlc *client.Client, tmpls *template.Template) http.HandlerFunc {
	indexTmpl := tmpls.Lookup("index.html.tmpl")
	if indexTmpl == nil {
		log.Fatal("missing template: index.html.tmpl")
	}

	return func(w http.ResponseWriter, r *http.Request) {
		data, err := fetchIndexData(r.Context(), dlc)
		if err != nil {
			error500(log, w, "fetch index data", err)
			return
		}

		indexTmpl.Execute(w, data)
	}
}

type Object struct {
	Path      string
	Mode      string
	Size      string
	Truncated bool
	Content   string
}

type VersionData struct {
	Project  int64
	Version  int64
	Versions []int64
	Objects  []Object
}

func fetchVersionData(ctx context.Context, dlc *client.Client, project, version int64) (*VersionData, error) {
	vrange := client.VersionRange{From: nil, To: &version}
	get, err := dlc.Get(ctx, project, "", vrange)
	if err != nil {
		return nil, err
	}

	var objects []Object

	for _, object := range get {
		mode := fs.FileMode(object.Mode)
		truncated := len(object.Content) > 2000

		content := object.Content
		if truncated {
			content = content[:2000]
		}

		objects = append(objects, Object{
			Path:      object.Path,
			Mode:      mode.String(),
			Size:      fmt.Sprintf("%.3f KB", (float64(object.Size) / 1000)),
			Truncated: truncated,
			Content:   html.EscapeString(string(content)),
		})
	}

	inspect, err := dlc.Inspect(ctx, project)
	if err != nil {
		return nil, err
	}

	versions := make([]int64, inspect.LatestVersion)
	for i := int64(1); i <= inspect.LatestVersion; i++ {
		versions[i-1] = i
	}

	return &VersionData{
		Project:  project,
		Version:  version,
		Versions: versions,
		Objects:  objects,
	}, nil
}

func getVersion(log *zap.Logger, dlc *client.Client, tmpls *template.Template) http.HandlerFunc {
	versionTmpl := tmpls.Lookup("version.html.tmpl")
	if versionTmpl == nil {
		log.Fatal("missing template: version.html.tmpl")
	}

	return func(w http.ResponseWriter, r *http.Request) {
		projectId, err := strconv.ParseInt(chi.URLParam(r, "projectId"), 10, 64)
		if err != nil {
			error400(log, w, "invalid projectId", err, zap.String("projectId", chi.URLParam(r, "projectId")))
			return
		}
		version, err := strconv.ParseInt(chi.URLParam(r, "version"), 10, 64)
		if err != nil {
			error400(log, w, "invalid version", err, zap.String("version", chi.URLParam(r, "version")))
			return
		}

		data, err := fetchVersionData(r.Context(), dlc, projectId, version)
		if err != nil {
			error500(log, w, "fetch version data", err)
			return
		}

		versionTmpl.Execute(w, data)
	}
}
