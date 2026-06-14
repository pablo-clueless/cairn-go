package http

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	"cairn/internal/auth"
	"cairn/internal/config"
	"cairn/internal/email"
	"cairn/internal/org"
	"cairn/internal/store"
)

// Server holds shared dependencies for HTTP handlers.
type Server struct {
	db    *store.DB
	cfg   config.Config
	auth  *auth.Service
	oauth *auth.OAuth
	orgs  *org.Service
}

// NewServer constructs a Server with its dependencies.
func NewServer(db *store.DB, cfg config.Config) *Server {
	mailer := email.New(cfg.SMTP)
	return &Server{
		db:    db,
		cfg:   cfg,
		auth:  auth.NewService(db, cfg),
		oauth: auth.NewOAuth(cfg),
		orgs:  org.NewService(db, mailer, cfg.FrontendURL, cfg.InviteTTL),
	}
}

// Router builds the chi router with global middleware and routes.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(s.cors)

	// Liveness/readiness probe.
	r.Get("/healthz", s.handleHealth)

	// Interactive API docs at /swagger/index.html
	r.Get("/swagger/*", httpSwagger.WrapHandler)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", s.handleHealth)

		r.Route("/auth", func(r chi.Router) {
			r.Post("/signup", s.handleSignup)
			r.Post("/login", s.handleLogin)
			r.Post("/refresh", s.handleRefresh)
			r.Post("/logout", s.handleLogout)

			// SSO (Google / Microsoft)
			r.Get("/oauth/{provider}", s.handleOAuthLogin)
			r.Get("/oauth/{provider}/callback", s.handleOAuthCallback)

			r.Group(func(r chi.Router) {
				r.Use(s.authenticate)
				r.Get("/me", s.handleMe)
			})
		})

		// Authenticated, org-scoped routes.
		r.Group(func(r chi.Router) {
			r.Use(s.authenticate)

			r.Post("/invitations/accept", s.handleAcceptInvite)

			r.Route("/orgs", func(r chi.Router) {
				r.Post("/", s.handleCreateOrg)
				r.Get("/", s.handleListOrgs)

				r.Route("/{orgID}", func(r chi.Router) {
					r.Use(s.orgContext)

					r.Get("/", s.handleGetOrg)
					r.Patch("/", s.handleUpdateOrg)

					r.Get("/members", s.handleListMembers)
					r.Patch("/members/{userID}", s.handleUpdateMemberRole)
					r.Delete("/members/{userID}", s.handleRemoveMember)

					r.Get("/invitations", s.handleListInvites)
					r.Post("/invitations", s.handleCreateInvite)
					r.Delete("/invitations/{inviteID}", s.handleDeleteInvite)
				})
			})
		})
		// Phase 3+ routes (projects, issues) mount under /orgs/{orgID}.
	})

	return r
}
