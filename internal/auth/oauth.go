package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/microsoft"

	"cairn/internal/config"
)

// Provider names for federated login.
const (
	ProviderGoogle    = "google"
	ProviderMicrosoft = "microsoft"
)

// OAuthUser is the normalized identity returned by a provider.
type OAuthUser struct {
	Sub   string
	Email string
	Name  string
}

type oauthProvider struct {
	config      *oauth2.Config
	userInfoURL string
}

// OAuth manages configured SSO providers.
type OAuth struct {
	providers map[string]oauthProvider
}

// NewOAuth builds the OAuth registry from config. A provider is registered only
// when its client id and secret are present.
func NewOAuth(cfg config.Config) *OAuth {
	o := &OAuth{providers: map[string]oauthProvider{}}
	base := cfg.AppBaseURL + "/v1/auth/oauth/"

	if cfg.OAuth.GoogleClientID != "" && cfg.OAuth.GoogleClientSecret != "" {
		o.providers[ProviderGoogle] = oauthProvider{
			config: &oauth2.Config{
				ClientID:     cfg.OAuth.GoogleClientID,
				ClientSecret: cfg.OAuth.GoogleClientSecret,
				RedirectURL:  base + ProviderGoogle + "/callback",
				Endpoint:     google.Endpoint,
				Scopes:       []string{"openid", "email", "profile"},
			},
			userInfoURL: "https://openidconnect.googleapis.com/v1/userinfo",
		}
	}

	if cfg.OAuth.MicrosoftClientID != "" && cfg.OAuth.MicrosoftClientSecret != "" {
		tenant := cfg.OAuth.MicrosoftTenant
		if tenant == "" {
			tenant = "common"
		}
		o.providers[ProviderMicrosoft] = oauthProvider{
			config: &oauth2.Config{
				ClientID:     cfg.OAuth.MicrosoftClientID,
				ClientSecret: cfg.OAuth.MicrosoftClientSecret,
				RedirectURL:  base + ProviderMicrosoft + "/callback",
				Endpoint:     microsoft.AzureADEndpoint(tenant),
				Scopes:       []string{"openid", "email", "profile"},
			},
			userInfoURL: "https://graph.microsoft.com/oidc/userinfo",
		}
	}

	return o
}

// Enabled reports whether a provider is configured.
func (o *OAuth) Enabled(provider string) bool {
	_, ok := o.providers[provider]
	return ok
}

// AuthCodeURL returns the provider's consent URL for the given CSRF state.
func (o *OAuth) AuthCodeURL(provider, state string) (string, bool) {
	p, ok := o.providers[provider]
	if !ok {
		return "", false
	}
	return p.config.AuthCodeURL(state, oauth2.AccessTypeOffline), true
}

// Exchange swaps an authorization code for tokens.
func (o *OAuth) Exchange(ctx context.Context, provider, code string) (*oauth2.Token, error) {
	p, ok := o.providers[provider]
	if !ok {
		return nil, fmt.Errorf("auth: unknown provider %q", provider)
	}
	return p.config.Exchange(ctx, code)
}

// UserInfo fetches and normalizes the provider's userinfo for a token.
func (o *OAuth) UserInfo(ctx context.Context, provider string, token *oauth2.Token) (*OAuthUser, error) {
	p, ok := o.providers[provider]
	if !ok {
		return nil, fmt.Errorf("auth: unknown provider %q", provider)
	}

	client := p.config.Client(ctx, token)
	resp, err := client.Get(p.userInfoURL)
	if err != nil {
		return nil, fmt.Errorf("auth: fetch userinfo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth: userinfo status %d", resp.StatusCode)
	}

	var payload struct {
		Sub               string `json:"sub"`
		Email             string `json:"email"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("auth: decode userinfo: %w", err)
	}

	email := payload.Email
	if email == "" {
		email = payload.PreferredUsername // Microsoft often returns the UPN here.
	}
	return &OAuthUser{Sub: payload.Sub, Email: email, Name: payload.Name}, nil
}
