package web

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"

	"readmecheck/internal/github"
	"readmecheck/internal/rules"
	"readmecheck/internal/store"
	"readmecheck/internal/web/views"
)

type Config struct {
	AssetsDir     string
	DatabasePath  string
	GitHubToken   string
	GitHubAPIURL  string
	Provider      github.Provider
	History       store.Store
	LinkChecker   rules.LinkChecker
	Now           func() time.Time
	ExternalLinks bool
}

type Server struct {
	assetsDir   string
	provider    github.Provider
	history     store.Store
	ownsHistory bool
	linkChecker rules.LinkChecker
	now         func() time.Time
	external    bool
	mux         *http.ServeMux
}

func New(cfg Config) (*Server, error) {
	if cfg.AssetsDir == "" {
		cfg.AssetsDir = "assets"
	}
	if cfg.DatabasePath == "" {
		cfg.DatabasePath = "readmecheck.db"
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.LinkChecker == nil {
		cfg.LinkChecker = rules.NewHTTPLinkChecker()
	}

	provider := cfg.Provider
	if provider == nil {
		client, err := github.NewClient(github.ClientOptions{
			Token:   cfg.GitHubToken,
			BaseURL: cfg.GitHubAPIURL,
		})
		if err != nil {
			return nil, err
		}
		provider = client
	}

	history := cfg.History
	ownsHistory := false
	if history == nil {
		sqliteStore, err := store.Open(cfg.DatabasePath)
		if err != nil {
			return nil, err
		}
		history = sqliteStore
		ownsHistory = true
	}

	s := &Server{
		assetsDir:   cfg.AssetsDir,
		provider:    provider,
		history:     history,
		ownsHistory: ownsHistory,
		linkChecker: cfg.LinkChecker,
		now:         cfg.Now,
		external:    cfg.ExternalLinks,
		mux:         http.NewServeMux(),
	}
	s.routes()
	return s, nil
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		s.mux.ServeHTTP(w, r)
	})
}

func (s *Server) Close() error {
	if s.ownsHistory {
		return s.history.Close()
	}
	return nil
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /{$}", s.home)
	s.mux.HandleFunc("POST /check", s.check)
	s.mux.HandleFunc("GET /report/{owner}/{repo}", s.report)
	s.mux.HandleFunc("GET /history", s.historyPage)
	s.mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(s.assetsDir))))
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	data := views.HomeData{
		Title:         "Audit your README",
		URL:           r.URL.Query().Get("url"),
		ExternalLinks: s.external,
	}
	s.render(w, r, http.StatusOK, views.Home(data))
}

func (s *Server) check(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		s.handleInputError(w, r, "", s.external, "The form could not be read.")
		return
	}
	rawURL := strings.TrimSpace(r.PostFormValue("url"))
	external := formBool(r.PostFormValue("external"))
	isHTMX := isHTMX(r)

	ref, err := github.ParseRepositoryURL(rawURL)
	if err != nil {
		message := err.Error()
		if !isHTMX {
			data := views.HomeData{Title: "Audit your README", URL: rawURL, ExternalLinks: external, Error: message}
			s.render(w, r, http.StatusUnprocessableEntity, views.Home(data))
			return
		}
		s.render(w, r, http.StatusOK, views.CheckError(message))
		return
	}

	report, err := s.analyze(r.Context(), ref, external)
	if err != nil {
		message := friendlyFetchError(err)
		if isHTMX {
			s.render(w, r, http.StatusOK, views.CheckError(message))
			return
		}
		data := views.HomeData{Title: "Audit your README", URL: rawURL, ExternalLinks: external, Error: message}
		s.render(w, r, statusForError(err), views.Home(data))
		return
	}

	s.record(r.Context(), report)
	data := views.NewReportData(report, external)
	if isHTMX {
		w.Header().Set("HX-Push-Url", reportPath(ref, external))
		s.render(w, r, http.StatusOK, views.ReportResult(data))
		return
	}
	s.render(w, r, http.StatusOK, views.ReportPage(data))
}

func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	ref, err := github.ParseRepositoryURL(r.PathValue("owner") + "/" + r.PathValue("repo"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	external := queryBool(r.URL.Query().Get("external"))
	report, err := s.analyze(r.Context(), ref, external)
	if err != nil {
		data := views.HomeData{
			Title:         "Audit your README",
			URL:           ref.URL(),
			ExternalLinks: external,
			Error:         friendlyFetchError(err),
		}
		s.render(w, r, statusForError(err), views.Home(data))
		return
	}
	s.record(r.Context(), report)
	s.render(w, r, http.StatusOK, views.ReportPage(views.NewReportData(report, external)))
}

func (s *Server) historyPage(w http.ResponseWriter, r *http.Request) {
	checks, err := s.history.Recent(r.Context(), 50)
	if err != nil {
		slog.Error("load history", "error", err)
		s.render(w, r, http.StatusInternalServerError, views.History(views.HistoryData{
			Error: "Check history could not be loaded.",
		}))
		return
	}
	summaries := make([]views.CheckSummary, 0, len(checks))
	for _, check := range checks {
		summaries = append(summaries, views.CheckSummary{
			Repository:    check.Owner + "/" + check.Repo,
			Description:   check.Description,
			License:       check.License,
			Stars:         check.Stars,
			Score:         check.Score,
			Grade:         check.Grade,
			CheckedAt:     check.CheckedAt,
			ExternalLinks: check.ExternalLinks,
			Path:          "/report/" + check.Owner + "/" + check.Repo,
		})
	}
	s.render(w, r, http.StatusOK, views.History(views.HistoryData{Checks: summaries}))
}

func (s *Server) analyze(ctx context.Context, ref github.RepositoryRef, external bool) (rules.Report, error) {
	repository, err := s.provider.Fetch(ctx, ref)
	if err != nil {
		return rules.Report{}, err
	}
	return rules.Analyze(ctx, repository, rules.Options{
		CheckExternalLinks: external,
		LinkChecker:        s.linkChecker,
		Now:                s.now,
	}), nil
}

func (s *Server) record(ctx context.Context, report rules.Report) {
	repository := report.Repository
	license := ""
	if repository.License != nil {
		license = repository.License.Name
	}
	check := store.Check{
		Owner:         repository.Ref.Owner,
		Repo:          repository.Ref.Repo,
		Description:   repository.Description,
		License:       license,
		Stars:         repository.Stars,
		Score:         report.Score,
		Grade:         report.Grade,
		CheckedAt:     report.GeneratedAt,
		ExternalLinks: report.ExternalLinksChecked,
	}
	if err := s.history.Save(ctx, check); err != nil {
		slog.Error("save check history", "error", err, "repository", repository.FullName)
	}
}

func (s *Server) handleInputError(w http.ResponseWriter, r *http.Request, rawURL string, external bool, message string) {
	if isHTMX(r) {
		s.render(w, r, http.StatusOK, views.CheckError(message))
		return
	}
	s.render(w, r, http.StatusUnprocessableEntity, views.Home(views.HomeData{
		Title:         "Audit your README",
		URL:           rawURL,
		ExternalLinks: external,
		Error:         message,
	}))
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, component templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("render page", "error", err, "path", r.URL.Path)
	}
}

func reportPath(ref github.RepositoryRef, external bool) string {
	target := "/report/" + ref.Owner + "/" + ref.Repo
	if external {
		target += "?external=1"
	}
	return target
}

func isHTMX(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("HX-Request"), "true")
}

func formBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

func queryBool(value string) bool {
	return formBool(value)
}

func statusForError(err error) int {
	if errors.Is(err, github.ErrNotFound) {
		return http.StatusNotFound
	}
	var rateLimit *github.RateLimitError
	if errors.As(err, &rateLimit) {
		return http.StatusTooManyRequests
	}
	var httpErr *github.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusUnauthorized {
		return http.StatusBadGateway
	}
	return http.StatusBadGateway
}

func friendlyFetchError(err error) string {
	var rateLimit *github.RateLimitError
	if errors.As(err, &rateLimit) {
		if !rateLimit.Reset.IsZero() {
			return fmt.Sprintf("GitHub's API rate limit has been reached. It resets at %s. Set GITHUB_TOKEN to raise the limit to 5,000 requests per hour.", rateLimit.Reset.Local().Format("15:04 MST"))
		}
		return "GitHub's API rate limit has been reached. Try again later or set GITHUB_TOKEN to raise the limit."
	}
	if errors.Is(err, github.ErrNotFound) {
		return "Repository not found. Check the URL for typos. Private repositories require a GITHUB_TOKEN with access to the repository."
	}
	var httpErr *github.HTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.StatusCode {
		case http.StatusUnauthorized:
			return "GitHub rejected the access token. Check GITHUB_TOKEN and try again."
		case http.StatusForbidden:
			return "GitHub refused access to this repository. A GITHUB_TOKEN with repository access may help."
		default:
			return fmt.Sprintf("GitHub returned an unexpected response (HTTP %d). Try again shortly.", httpErr.StatusCode)
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "GitHub took too long to respond. Try again in a moment."
	}
	if errors.Is(err, context.Canceled) {
		return "The request was canceled. Try again."
	}
	return "The repository could not be loaded. Check your connection and try again."
}
