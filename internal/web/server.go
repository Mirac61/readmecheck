package web

import (
	"log/slog"
	"net/http"

	"github.com/a-h/templ"

	"readmecheck/internal/web/views"
)

type Config struct {
	AssetsDir string
}

type Server struct {
	assetsDir string
	mux       *http.ServeMux
}

func New(cfg Config) (*Server, error) {
	if cfg.AssetsDir == "" {
		cfg.AssetsDir = "assets"
	}

	s := &Server{
		assetsDir: cfg.AssetsDir,
		mux:       http.NewServeMux(),
	}
	s.routes()
	return s, nil
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /{$}", s.home)
	s.mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(s.assetsDir))))
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	data := views.HomeData{
		Title: "Audit your README",
		URL:   r.URL.Query().Get("url"),
	}
	s.render(w, r, http.StatusOK, views.Home(data))
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, component templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("render page", "error", err, "path", r.URL.Path)
	}
}
