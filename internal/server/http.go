package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-family/internal/store"
)

// Router builds the chi router.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	if s.cfg.TrustProxy {
		r.Use(middleware.RealIP)
	}
	r.Use(middleware.RequestID)
	r.Use(s.logging)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	r.Get("/pocket-ask.js", s.core.AskWidget("family"))
	r.Get("/healthz", s.handleHealth)
	r.Get("/readyz", s.handleReady)
	r.Get("/api/health", s.handleHealth)
	if s.cfg.MetricsEnabled {
		r.Get("/metrics", s.handleMetrics)
	}

	r.Route("/api/v1", func(r chi.Router) {
		// exports, imports and AI can take longer than a request may
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Use(s.needFamily)
			r.Get("/export.json", s.handleExportJSON)
			r.Post("/import", s.handleImport)
			r.Post("/ai/letter", s.handleAILetter)
			r.Post("/ai/quick", s.handleAIQuick)
			r.Get("/ai/week", s.handleAIWeek)
		})

		r.Group(func(r chi.Router) {
			r.Use(middleware.Timeout(120 * time.Second))
			r.Post("/auth/login", s.handleLogin)
			r.Post("/auth/setup", s.handleSetup)
			r.Get("/auth/status", s.handleAuthStatus)
			r.Get("/auth/oidc/start", s.handleOIDCStart)
			r.Get("/auth/oidc/callback", s.handleOIDCCallback)

			r.Group(func(r chi.Router) {
				r.Use(s.requireAuth)
				r.Post("/auth/logout", s.handleLogout)
				r.Get("/auth/me", s.handleMe)
				r.Put("/auth/me", s.handleUpdateMe)

				// families (before one is chosen)
				r.Get("/families", s.handleFamilies)
				r.Post("/families", s.handleCreateFamily)
				r.Post("/families/switch", s.handleSwitchFamily)

				// AI, notifications, people on the box, settings, tokens, backup
				r.Get("/ai/status", s.handleAIStatus)
				r.Get("/ai/models", s.handleAIModels)
				r.Post("/integrations/test", s.handleIntegrationTest)
				r.Post("/notify/test", s.handleNotifyTest)
				r.Post("/notify/check", s.handleCheckNow)
				r.Get("/apps", s.handleApps)

				r.Get("/users", s.handleListUsers)
				r.Post("/users", s.handleCreateUser)
				r.Put("/users/{id}", s.handleUpdateUser)
				r.Delete("/users/{id}", s.handleDeleteUser)

				r.Get("/settings", s.handleGetSettings)
				r.Put("/settings", s.handlePutSettings)
				r.Get("/settings/system", s.handleSystemInfo)

				r.Get("/tokens", s.handleListTokens)
				r.Post("/tokens", s.handleCreateToken)
				r.Delete("/tokens/{id}", s.handleDeleteToken)

				r.Get("/backup", s.handleBackup)

				// everything in a family
				r.Group(func(r chi.Router) {
					r.Use(s.needFamily)
					r.Get("/family", s.handleGetFamily)
					r.Put("/family", s.handleUpdateFamily)
					r.Delete("/family", s.handleDeleteFamily)
					r.Post("/family/leave", s.handleLeaveFamily)
					r.Post("/people", s.handleCreatePerson)
					r.Put("/people/{id}", s.handleUpdatePerson)
					r.Delete("/people/{id}", s.handleDeletePerson)
					r.Get("/activity", s.handleActivity)

					// the day, the week, for the rest of the box
					r.Get("/today", s.handleToday)
					r.Get("/week", s.handleWeek)
					r.Get("/day/{date}", s.handleDay)
					r.Get("/summary", s.handleSummary)
					r.Get("/search", s.handleSearch)

					// pick-ups
					r.Get("/runs", s.handleRuns)
					r.Post("/runs", s.handleCreateRun)
					r.Put("/runs/{id}", s.handleUpdateRun)
					r.Delete("/runs/{id}", s.handleDeleteRun)
					r.Post("/runs/{id}/{action}", s.handleRunAction)
					r.Get("/plans", s.handlePlans)
					r.Post("/plans", s.handleCreatePlan)
					r.Put("/plans/{id}", s.handleUpdatePlan)
					r.Delete("/plans/{id}", s.handleDeletePlan)

					// chores
					r.Get("/chores", s.handleChores)
					r.Post("/chores", s.handleCreateChore)
					r.Put("/chores/{id}", s.handleUpdateChore)
					r.Delete("/chores/{id}", s.handleDeleteChore)
					r.Post("/chores/{id}/done", s.handleChoreDone)
					r.Post("/chores/{id}/undo", s.handleChoreUndo)
					r.Get("/stars", s.handleStars)

					// plans
					r.Get("/events", s.handleEvents)
					r.Post("/events", s.handleCreateEvent)
					r.Get("/events/{id}", s.handleGetEvent)
					r.Put("/events/{id}", s.handleUpdateEvent)
					r.Delete("/events/{id}", s.handleDeleteEvent)

					// shopping
					r.Get("/lists", s.handleLists)
					r.Post("/lists", s.handleCreateList)
					r.Put("/lists/{id}", s.handleUpdateList)
					r.Delete("/lists/{id}", s.handleDeleteList)
					r.Get("/lists/{id}/items", s.handleItems)
					r.Post("/lists/{id}/items", s.handleAddItem)
					r.Post("/lists/{id}/shopping", s.handleShopping)
					r.Post("/lists/{id}/clear", s.handleClearDone)
					r.Post("/lists/{id}/from-plan", s.handleFromPlan)
					r.Post("/items", s.handleQuickItem)
					r.Get("/items/recent", s.handleRecentItems)
					r.Put("/items/{id}", s.handleUpdateItem)
					r.Delete("/items/{id}", s.handleDeleteItem)

					// school
					r.Get("/school", s.handleSchool)
					r.Post("/terms", s.handleSaveTerm)
					r.Put("/terms/{id}", s.handleSaveTerm)
					r.Delete("/terms/{id}", s.handleDeleteTerm)
					r.Put("/school/week/{person}", s.handleSchoolWeek)
					r.Get("/letters", s.handleLetters)
					r.Post("/letters", s.handleCreateLetter)
					r.Get("/letters/{id}", s.handleGetLetter)
					r.Put("/letters/{id}", s.handleUpdateLetter)
					r.Delete("/letters/{id}", s.handleDeleteLetter)

					// the board and the family info
					r.Get("/posts", s.handlePosts)
					r.Post("/posts", s.handleCreatePost)
					r.Put("/posts/{id}", s.handleUpdatePost)
					r.Delete("/posts/{id}", s.handleDeletePost)
					r.Post("/posts/seen", s.handleSeePosts)
					r.Get("/info", s.handleInfo)
					r.Post("/info", s.handleSaveInfo)
					r.Put("/info/{id}", s.handleSaveInfo)
					r.Delete("/info/{id}", s.handleDeleteInfo)

					// the other Pocket apps
					r.Post("/make", s.handleMake)
					r.Get("/made", s.handleMadeList)
					r.Get("/make/options", s.handleMakeOptions)
					r.Get("/make/accounts", s.handleMakeAccounts)
					r.Get("/make/groups", s.handleMakeGroups)
					r.Get("/make/chats", s.handleChats)
					r.Get("/pick/contacts", s.handlePickContacts)
				})
			})
		})
	})

	r.NotFound(s.handleSPA)
	return r
}

// ---------- helpers ----------

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.requests.Add(1)
		if ww.Status() >= 500 {
			s.errors.Add(1)
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			return
		}
		s.log.Debug("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "dur", time.Since(start).String(), "ip", r.RemoteAddr)
	})
}

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

func (s *Server) fail(w http.ResponseWriter, err error, context string) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
		return
	case errors.Is(err, store.ErrConflict), errors.Is(err, core.ErrOperationConflict):
		writeErr(w, http.StatusConflict, "already exists")
		return
	}
	s.log.Error(context, "err", err)
	writeErr(w, http.StatusInternalServerError, context+": "+err.Error())
}

func (s *Server) readBody(r *http.Request) ([]byte, error) {
	return io.ReadAll(http.MaxBytesReader(nil, r.Body, s.cfg.MaxBodyBytes))
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 32<<20))
	return dec.Decode(v)
}

// decodeKeys decodes JSON into v and also reports which top-level keys were
// present, so PUT handlers can tell "not sent" from "sent empty".
func decodeKeys(r *http.Request, v any) (map[string]bool, error) {
	b, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	keys := make(map[string]bool, len(raw))
	for k := range raw {
		keys[k] = true
	}
	return keys, json.Unmarshal(b, v)
}

func idParam(r *http.Request, name string) int64 {
	n, _ := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return n
}

func queryInt(r *http.Request, name string) int {
	n, _ := strconv.Atoi(r.URL.Query().Get(name))
	return n
}

func queryBool(r *http.Request, name string) bool {
	v := strings.ToLower(r.URL.Query().Get(name))
	return v == "1" || v == "true" || v == "yes"
}

func validDate(s string) bool {
	if s == "" {
		return true
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// ---------- health / metrics ----------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"core_outbox": s.coreOutboxHealth(r.Context()), "status": "ok", "version": Version, "uptime": time.Since(s.started).Round(time.Second).String()})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DB.PingContext(r.Context()); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	counts := s.store.Counts(ctx)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP pocket_family_info Build info.\n# TYPE pocket_family_info gauge\npocket_family_info{version=%q} 1\n", Version)
	fmt.Fprintf(w, "# HELP pocket_family_uptime_seconds Process uptime.\n# TYPE pocket_family_uptime_seconds gauge\npocket_family_uptime_seconds %d\n", int(time.Since(s.started).Seconds()))
	fmt.Fprintf(w, "# HELP pocket_family_http_requests_total HTTP requests served.\n# TYPE pocket_family_http_requests_total counter\npocket_family_http_requests_total %d\n", s.requests.Load())
	fmt.Fprintf(w, "# HELP pocket_family_http_errors_total HTTP 5xx responses.\n# TYPE pocket_family_http_errors_total counter\npocket_family_http_errors_total %d\n", s.errors.Load())
	fmt.Fprintf(w, "# HELP pocket_family_rows Rows per table.\n# TYPE pocket_family_rows gauge\n")
	for k, v := range counts {
		fmt.Fprintf(w, "pocket_family_rows{table=%q} %d\n", k, v)
	}
}

// ---------- static / SPA ----------

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if s.webFS == nil {
		http.Error(w, "web UI not built (run `make web`)", http.StatusNotFound)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if f, err := s.webFS.Open(path); err == nil {
		f.Close()
		if strings.HasPrefix(path, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.FileServerFS(s.webFS).ServeHTTP(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	r.URL.Path = "/"
	http.FileServerFS(s.webFS).ServeHTTP(w, r)
}
