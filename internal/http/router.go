package http

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"cairn/docs"
	"cairn/internal/auth"
	"cairn/internal/billing"
	"cairn/internal/config"
	"cairn/internal/email"
	"cairn/internal/org"
	"cairn/internal/realtime"
	"cairn/internal/store"
	"cairn/internal/work"
)

// Server holds shared dependencies for HTTP handlers.
type Server struct {
	db      *store.DB
	cfg     config.Config
	auth    *auth.Service
	oauth   *auth.OAuth
	orgs    *org.Service
	billing *billing.Service
	work    *work.Service
	mailer  *email.Sender
	rt      realtime.Broadcaster
	hub     *realtime.Hub
}

// NewServer constructs a Server with its dependencies.
func NewServer(db *store.DB, cfg config.Config) *Server {
	mailer := email.New(cfg.SMTP)
	hub := realtime.NewHub(cfg.FrontendURL)
	return &Server{
		db:      db,
		cfg:     cfg,
		auth:    auth.NewService(db, cfg),
		oauth:   auth.NewOAuth(cfg),
		orgs:    org.NewService(db, mailer, cfg.FrontendURL, cfg.InviteTTL),
		billing: billing.NewService(db, cfg.DefaultPricePerSeatCents, cfg.DefaultCurrency),
		work:    work.NewService(db, mailer, cfg.FrontendURL, cfg.AttachmentsDir, cfg.MaxUploadBytes),
		mailer:  mailer,
		rt:      hub,
		hub:     hub,
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

	// Socket.IO realtime endpoint (auth handled inline from the cookie).
	r.HandleFunc("/socket.io/*", s.handleSocketIO)

	// Interactive API docs at /swagger/index.html.
	// We generate an OpenAPI 3.1 document, but the Swagger UI bundled with
	// http-swagger/v2 is 3.x and can't render 3.1, and it reads from the swag v1
	// registry rather than the swag/v2 registry our docs register into. So serve
	// the spec ourselves and render it with Swagger UI 5 (3.1-capable).
	r.Get("/swagger/doc.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(docs.SwaggerInfo.ReadDoc()))
	})
	r.Get("/swagger/*", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(swaggerUIPage))
	})

	r.Route("/v1", func(r chi.Router) {
		r.Get("/health", s.handleHealth)

		r.Route("/auth", func(r chi.Router) {
			r.Post("/signup", s.handleSignup)
			r.Post("/login", s.handleLogin)
			r.Post("/refresh", s.handleRefresh)
			r.Post("/logout", s.handleLogout)
			r.Post("/forgot-password", s.handleForgotPassword)
			r.Post("/reset-password", s.handleResetPassword)

			// SSO (Google / Microsoft)
			r.Get("/oauth/{provider}", s.handleOAuthLogin)
			r.Get("/oauth/{provider}/callback", s.handleOAuthCallback)
		})

		// Authenticated, org-scoped routes.
		r.Group(func(r chi.Router) {
			r.Use(s.authenticate)

			r.Get("/me", s.handleMe)
			r.Patch("/invitations/{token}", s.handleAcceptInvite)

			// Personal notification inbox (cross-org).
			r.Get("/notifications", s.handleListNotifications)
			r.Patch("/notifications", s.handleMarkAllRead)
			r.Get("/notifications/count", s.handleNotificationCount)
			r.Get("/notifications/preferences", s.handleGetNotificationPrefs)
			r.Patch("/notifications/preferences", s.handleUpdateNotificationPrefs)
			r.Patch("/notifications/{id}", s.handleMarkNotificationRead)

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

					r.Get("/subscription", s.handleGetSubscription)

					r.Get("/search", s.handleSearch)

					r.Get("/filters", s.handleListFilters)
					r.Post("/filters", s.handleCreateFilter)
					r.Patch("/filters/{filterID}", s.handleUpdateFilter)
					r.Delete("/filters/{filterID}", s.handleDeleteFilter)

					r.Get("/dashboards", s.handleListDashboards)
					r.Post("/dashboards", s.handleCreateDashboard)
					r.Patch("/dashboards/{dashboardID}", s.handleUpdateDashboard)
					r.Delete("/dashboards/{dashboardID}", s.handleDeleteDashboard)

					// Spaces (projects) & issues
					r.Get("/spaces", s.handleListSpaces)
					r.Post("/spaces", s.handleCreateSpace)
					r.Get("/spaces/{spaceKey}", s.handleGetSpace)
					r.Patch("/spaces/{spaceKey}", s.handleUpdateSpace)
					r.Delete("/spaces/{spaceKey}", s.handleDeleteSpace)

					r.Get("/spaces/{spaceKey}/members", s.handleListSpaceMembers)
					r.Post("/spaces/{spaceKey}/members", s.handleAddSpaceMember)
					r.Delete("/spaces/{spaceKey}/members/{userID}", s.handleRemoveSpaceMember)

					r.Get("/spaces/{spaceKey}/invitations", s.handleListSpaceInvitations)
					r.Post("/spaces/{spaceKey}/invitations", s.handleInviteToSpace)
					r.Delete("/spaces/{spaceKey}/invitations/{inviteID}", s.handleDeleteSpaceInvitation)
					r.Post("/spaces/{spaceKey}/issues", s.handleCreateIssue)

					r.Get("/spaces/{spaceKey}/statuses", s.handleListStatuses)
					r.Post("/spaces/{spaceKey}/statuses", s.handleCreateStatus)
					r.Patch("/spaces/{spaceKey}/statuses", s.handleBulkUpdateStatuses)
					r.Patch("/statuses/{statusID}", s.handleUpdateStatus)
					r.Delete("/statuses/{statusID}", s.handleDeleteStatus)

					r.Get("/spaces/{spaceKey}/transitions", s.handleListTransitions)
					r.Put("/spaces/{spaceKey}/transitions", s.handleSetTransitions)

					r.Get("/spaces/{spaceKey}/reports/velocity", s.handleVelocity)
					r.Get("/spaces/{spaceKey}/reports/burndown", s.handleBurndown)
					r.Get("/spaces/{spaceKey}/reports/cfd", s.handleCFD)

					r.Get("/spaces/{spaceKey}/sprints", s.handleListSprints)
					r.Post("/spaces/{spaceKey}/sprints", s.handleCreateSprint)
					r.Get("/sprints/{sprintID}", s.handleGetSprint)
					r.Patch("/sprints/{sprintID}", s.handleUpdateSprint)
					r.Delete("/sprints/{sprintID}", s.handleDeleteSprint)

					r.Get("/issues", s.handleListIssues)
					r.Get("/issues/{issueKey}", s.handleGetIssue)
					r.Patch("/issues/{issueKey}", s.handleUpdateIssue)
					r.Delete("/issues/{issueKey}", s.handleDeleteIssue)

					r.Get("/issues/{issueKey}/comments", s.handleListComments)
					r.Post("/issues/{issueKey}/comments", s.handleCreateComment)
					r.Patch("/comments/{commentID}", s.handleUpdateComment)
					r.Delete("/comments/{commentID}", s.handleDeleteComment)

					r.Get("/issues/{issueKey}/links", s.handleListLinks)
					r.Post("/issues/{issueKey}/links", s.handleCreateLink)
					r.Delete("/links/{linkID}", s.handleDeleteLink)

					r.Get("/issues/{issueKey}/watchers", s.handleListWatchers)
					r.Post("/issues/{issueKey}/watchers", s.handleWatchIssue)
					r.Delete("/issues/{issueKey}/watchers/{userID}", s.handleUnwatchIssue)
					r.Get("/issues/{issueKey}/activity", s.handleIssueActivity)

					r.Get("/issues/{issueKey}/attachments", s.handleListAttachments)
					r.Post("/issues/{issueKey}/attachments", s.handleUploadAttachment)
					r.Get("/attachments/{attachmentID}", s.handleDownloadAttachment)
					r.Delete("/attachments/{attachmentID}", s.handleDeleteAttachment)

					// Documents (space pages & live docs)
					r.Get("/spaces/{spaceKey}/documents", s.handleListDocuments)
					r.Post("/spaces/{spaceKey}/documents", s.handleCreateDocument)
					r.Get("/documents/{docID}", s.handleGetDocument)
					r.Patch("/documents/{docID}", s.handleUpdateDocument)
					r.Delete("/documents/{docID}", s.handleDeleteDocument)
				})
			})

			// Platform-admin (super-admin) routes.
			r.Group(func(r chi.Router) {
				r.Use(s.authenticate)
				r.Use(s.requirePlatformAdmin)

				r.Get("/admin/settings", s.handleGetSettings)
				r.Patch("/admin/settings", s.handleUpdateSettings)
				r.Get("/admin/orgs", s.handleAdminListOrgs)
				r.Get("/admin/orgs/{orgID}", s.handleAdminGetOrg)
				r.Patch("/admin/orgs/{orgID}/subscription", s.handleAdminUpdateSubscription)
			})
		})
		// Phase 3+ routes (projects, issues) mount under /orgs/{orgID}.
	})

	return r
}

// swaggerUIPage renders the OpenAPI 3.1 document at /swagger/doc.json using
// Swagger UI 5, which (unlike the 3.x bundle in http-swagger/v2) supports 3.1.
const swaggerUIPage = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Cairn API — Swagger UI</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui-bundle.js" crossorigin></script>
  <script>
    window.onload = function () {
      window.ui = SwaggerUIBundle({
        url: "/swagger/doc.json",
        dom_id: "#swagger-ui",
        deepLinking: true,
        persistAuthorization: true,
      });
    };
  </script>
</body>
</html>`
