package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration, loaded from the environment.
type Config struct {
	Env         string // "development" | "production"
	Port        string
	DatabaseURL string
	CORSOrigin  string
	SMTP        SMTPConfig

	// Auth
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	CookieSecure    bool

	// URLs
	AppBaseURL  string // public URL of this API (for OAuth redirect URIs)
	FrontendURL string // public URL of the Next.js app

	// Organizations
	InviteTTL time.Duration

	// Billing
	PlatformAdminEmails      []string // bootstrapped as platform admins at startup
	DefaultPricePerSeatCents int
	DefaultCurrency          string

	// SSO
	OAuth OAuthConfig
}

// OAuthConfig holds SSO provider credentials.
type OAuthConfig struct {
	GoogleClientID        string
	GoogleClientSecret    string
	MicrosoftClientID     string
	MicrosoftClientSecret string
	MicrosoftTenant       string
}

// SMTPConfig holds outbound email settings (used from Phase 2 onward for invites).
type SMTPConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
}

// Load reads configuration from environment variables, validating required fields.
func Load() (Config, error) {
	cfg := Config{
		Env:         getEnv("APP_ENV", "development"),
		Port:        getEnv("PORT", "8000"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		CORSOrigin:  getEnv("CORS_ORIGIN", "http://localhost:3000"),
	}

	if cfg.DatabaseURL == "" {
		return cfg, fmt.Errorf("config: DATABASE_URL is required")
	}

	smtpPort, err := strconv.Atoi(getEnv("SMTP_PORT", "587"))
	if err != nil {
		return cfg, fmt.Errorf("config: invalid SMTP_PORT: %w", err)
	}

	cfg.SMTP = SMTPConfig{
		Host:     os.Getenv("SMTP_HOST"),
		Port:     smtpPort,
		User:     os.Getenv("SMTP_USER"),
		Password: os.Getenv("SMTP_PASSWORD"),
		From:     getEnv("SMTP_FROM", os.Getenv("SMTP_USER")),
	}

	cfg.JWTSecret = os.Getenv("JWT_SECRET")
	if cfg.JWTSecret == "" {
		return cfg, fmt.Errorf("config: JWT_SECRET is required")
	}

	cfg.AccessTokenTTL, err = getDuration("ACCESS_TOKEN_TTL", 15*time.Minute)
	if err != nil {
		return cfg, err
	}
	cfg.RefreshTokenTTL, err = getDuration("REFRESH_TOKEN_TTL", 30*24*time.Hour)
	if err != nil {
		return cfg, err
	}
	cfg.CookieSecure = getEnv("COOKIE_SECURE", "false") == "true"

	cfg.AppBaseURL = getEnv("APP_BASE_URL", "http://localhost:"+cfg.Port)
	cfg.FrontendURL = getEnv("FRONTEND_URL", cfg.CORSOrigin)

	cfg.InviteTTL, err = getDuration("INVITE_TTL", 7*24*time.Hour)
	if err != nil {
		return cfg, err
	}

	cfg.OAuth = OAuthConfig{
		GoogleClientID:        os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret:    os.Getenv("GOOGLE_CLIENT_SECRET"),
		MicrosoftClientID:     os.Getenv("MICROSOFT_CLIENT_ID"),
		MicrosoftClientSecret: os.Getenv("MICROSOFT_CLIENT_SECRET"),
		MicrosoftTenant:       getEnv("MICROSOFT_TENANT", "common"),
	}

	cfg.PlatformAdminEmails = parseEmails(os.Getenv("PLATFORM_ADMIN_EMAILS"))
	cfg.DefaultPricePerSeatCents, err = getInt("DEFAULT_PRICE_PER_SEAT_CENTS", 1200)
	if err != nil {
		return cfg, err
	}
	cfg.DefaultCurrency = getEnv("DEFAULT_CURRENCY", "NGN")

	return cfg, nil
}

// parseEmails splits a comma-separated list into lowercased, trimmed emails.
func parseEmails(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	emails := make([]string, 0, len(parts))
	for _, p := range parts {
		if e := strings.ToLower(strings.TrimSpace(p)); e != "" {
			emails = append(emails, e)
		}
	}
	return emails
}

func getInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("config: invalid %s: %w", key, err)
	}
	return n, nil
}

func getDuration(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("config: invalid %s: %w", key, err)
	}
	return d, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
