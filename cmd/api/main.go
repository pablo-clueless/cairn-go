package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"cairn/internal/config"
	httpapi "cairn/internal/http"
	"cairn/internal/store"
	"cairn/migrations"

	_ "cairn/docs" // generated Swagger docs (run `make docs`)
)

//	@title			Cairn API
//	@version		0.1.0
//	@description	Organization-wide project & issue tracking API (Jira-class, org-scoped).
//	@host			localhost:8000
//	@BasePath		/v1

//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				Type "Bearer" followed by a space and the access token.

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// Load .env in local development; ignored if the file is absent (prod uses real env).
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	slog.Info("connected to database")

	if err := db.Migrate(ctx, migrations.FS); err != nil {
		return err
	}
	slog.Info("migrations applied")

	// Bootstrap platform admins from configuration (idempotent).
	if err := db.SetPlatformAdminByEmails(ctx, cfg.PlatformAdminEmails); err != nil {
		return err
	}
	if len(cfg.PlatformAdminEmails) > 0 {
		slog.Info("platform admins synced", "count", len(cfg.PlatformAdminEmails))
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httpapi.NewServer(db, cfg).Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("http server listening", "port", cfg.Port, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("listen error", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
