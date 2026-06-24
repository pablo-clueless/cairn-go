package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"cairn/internal/config"
	"cairn/internal/model"
	"cairn/internal/store"
)

// Service implements authentication: signup, login, token rotation, logout.
type Service struct {
	store      *store.DB
	jwtSecret  []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// NewService builds an auth Service from the store and config.
func NewService(db *store.DB, cfg config.Config) *Service {
	return &Service{
		store:      db,
		jwtSecret:  []byte(cfg.JWTSecret),
		accessTTL:  cfg.AccessTokenTTL,
		refreshTTL: cfg.RefreshTokenTTL,
	}
}

// TokenPair is the result of a successful authentication.
type TokenPair struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string // raw; set in an httpOnly cookie, never persisted
	RefreshExpiresAt time.Time
}

// AccessTTL exposes the access-token lifetime (used for cookie/response metadata).
func (s *Service) AccessTTL() time.Duration { return s.accessTTL }

// Signup creates a user and immediately issues a token pair.
func (s *Service) Signup(ctx context.Context, email, name, password, userAgent string) (*model.User, *TokenPair, error) {
	email = strings.TrimSpace(email)
	name = strings.TrimSpace(name)

	hash, err := hashPassword(password)
	if err != nil {
		return nil, nil, err
	}

	user, err := s.store.CreateUser(ctx, email, name, hash)
	if err != nil {
		if errors.Is(err, store.ErrEmailTaken) {
			return nil, nil, ErrEmailTaken
		}
		return nil, nil, err
	}

	pair, err := s.issueTokens(ctx, user.ID, userAgent)
	if err != nil {
		return nil, nil, err
	}
	return user, pair, nil
}

// Login verifies credentials and issues a token pair.
func (s *Service) Login(ctx context.Context, email, password, userAgent string) (*model.User, *TokenPair, error) {
	user, err := s.store.GetUserByEmail(ctx, strings.TrimSpace(email))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Run a dummy compare to reduce timing-based user enumeration.
			checkPassword("$2a$10$invalidinvalidinvalidinvalidinvalidinvalidinvalidinv", password)
			return nil, nil, ErrInvalidCredentials
		}
		return nil, nil, err
	}
	if !checkPassword(user.PasswordHash, password) {
		return nil, nil, ErrInvalidCredentials
	}

	pair, err := s.issueTokens(ctx, user.ID, userAgent)
	if err != nil {
		return nil, nil, err
	}
	return user, pair, nil
}

// Refresh validates a raw refresh token, rotates it, and issues a new pair.
func (s *Service) Refresh(ctx context.Context, rawRefresh, userAgent string) (*model.User, *TokenPair, error) {
	if rawRefresh == "" {
		return nil, nil, ErrInvalidToken
	}
	hash := hashRefreshToken(rawRefresh)

	stored, err := s.store.GetRefreshToken(ctx, hash)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, ErrInvalidToken
		}
		return nil, nil, err
	}
	if !stored.Active(time.Now()) {
		return nil, nil, ErrInvalidToken
	}

	// Rotate: revoke the presented token before minting a replacement.
	if err := s.store.RevokeRefreshToken(ctx, hash); err != nil {
		return nil, nil, err
	}

	user, err := s.store.GetUserByID(ctx, stored.UserID)
	if err != nil {
		return nil, nil, err
	}

	pair, err := s.issueTokens(ctx, user.ID, userAgent)
	if err != nil {
		return nil, nil, err
	}
	return user, pair, nil
}

// LoginWithOAuth resolves a federated identity to a local user (creating and/or
// linking as needed) and issues a token pair.
func (s *Service) LoginWithOAuth(ctx context.Context, provider string, info *OAuthUser, userAgent string) (*model.User, *TokenPair, error) {
	if info.Sub == "" || info.Email == "" {
		return nil, nil, ErrInvalidCredentials
	}

	// 1) Already linked identity?
	user, err := s.store.GetUserByIdentity(ctx, provider, info.Sub)
	switch {
	case err == nil:
		// linked already
	case errors.Is(err, store.ErrNotFound):
		// 2) Existing local account with the same email? Link it. Otherwise create.
		user, err = s.store.GetUserByEmail(ctx, info.Email)
		if errors.Is(err, store.ErrNotFound) {
			name := info.Name
			if name == "" {
				name = info.Email
			}
			user, err = s.store.CreateUserSSO(ctx, info.Email, name)
		}
		if err != nil {
			return nil, nil, err
		}
		if err := s.store.LinkIdentity(ctx, user.ID, provider, info.Sub); err != nil {
			return nil, nil, err
		}
	default:
		return nil, nil, err
	}

	pair, err := s.issueTokens(ctx, user.ID, userAgent)
	if err != nil {
		return nil, nil, err
	}
	return user, pair, nil
}

// Logout revokes the presented refresh token. It is best-effort and idempotent.
func (s *Service) Logout(ctx context.Context, rawRefresh string) error {
	if rawRefresh == "" {
		return nil
	}
	return s.store.RevokeRefreshToken(ctx, hashRefreshToken(rawRefresh))
}

// passwordResetTTL bounds how long a reset link remains valid.
const passwordResetTTL = time.Hour

// RequestPasswordReset issues a single-use reset token for the account with the
// given email and returns the raw token plus the user, so the caller can email
// the link. If no account matches, it returns ("", nil, nil) — callers respond
// identically either way to avoid leaking which emails are registered.
func (s *Service) RequestPasswordReset(ctx context.Context, email string) (string, *model.User, error) {
	user, err := s.store.GetUserByEmail(ctx, strings.TrimSpace(email))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return "", nil, nil
		}
		return "", nil, err
	}

	// Reuse the opaque-token helpers: a high-entropy raw token is emailed; only
	// its SHA-256 hash is stored.
	raw, err := generateRefreshToken()
	if err != nil {
		return "", nil, err
	}
	if err := s.store.CreatePasswordResetToken(ctx, user.ID, hashRefreshToken(raw), time.Now().Add(passwordResetTTL)); err != nil {
		return "", nil, err
	}
	return raw, user, nil
}

// ResetPassword validates a raw reset token and, if it is still active, sets a
// new password, consumes the token, and revokes the user's existing sessions.
func (s *Service) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	if rawToken == "" {
		return ErrInvalidToken
	}
	hash := hashRefreshToken(rawToken)

	tok, err := s.store.GetPasswordResetToken(ctx, hash)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrInvalidToken
		}
		return err
	}
	if !tok.Active(time.Now()) {
		return ErrInvalidToken
	}

	newHash, err := hashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.store.UpdateUserPassword(ctx, tok.UserID, newHash); err != nil {
		return err
	}
	if err := s.store.MarkPasswordResetTokenUsed(ctx, hash); err != nil {
		return err
	}
	// Invalidate existing sessions so a previously issued refresh token can't
	// outlive the password change.
	return s.store.RevokeAllRefreshTokensForUser(ctx, tok.UserID)
}

// issueTokens mints an access token and a persisted refresh token.
func (s *Service) issueTokens(ctx context.Context, userID, userAgent string) (*TokenPair, error) {
	now := time.Now()

	access, accessExp, err := s.generateAccessToken(userID, now)
	if err != nil {
		return nil, err
	}

	rawRefresh, err := generateRefreshToken()
	if err != nil {
		return nil, err
	}
	refreshExp := now.Add(s.refreshTTL)
	if err := s.store.CreateRefreshToken(ctx, userID, hashRefreshToken(rawRefresh), refreshExp, userAgent); err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:      access,
		AccessExpiresAt:  accessExp,
		RefreshToken:     rawRefresh,
		RefreshExpiresAt: refreshExp,
	}, nil
}
