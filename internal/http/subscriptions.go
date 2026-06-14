package http

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"cairn/internal/billing"
	"cairn/internal/store"
)

// appSettingsDTO mirrors model.AppSettings for API docs.
type appSettingsDTO struct {
	DefaultTrialDays int       `json:"default_trial_days"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// handleGetSubscription returns the caller's org subscription (any member).
//
//	@Summary	Get organization subscription
//	@Tags		billing
//	@Produce	json
//	@Security	BearerAuth
//	@Param		orgID	path		string	true	"Organization ID"
//	@Success	200		{object}	billing.View
//	@Router		/orgs/{orgID}/subscription [get]
func (s *Server) handleGetSubscription(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.requireOrg(w, r)
	if !ok {
		return
	}
	view, err := s.billing.Get(r.Context(), scope.Org.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not load subscription")
		return
	}
	respond(w, http.StatusOK, view)
}

// ---- Platform admin ----

type updateSettingsRequest struct {
	DefaultTrialDays *int `json:"default_trial_days"`
}

// handleGetSettings returns global platform settings.
//
//	@Summary	Get platform settings
//	@Tags		admin
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	appSettingsDTO
//	@Router		/admin/settings [get]
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.billing.Settings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not load settings")
		return
	}
	respond(w, http.StatusOK, settings)
}

// handleUpdateSettings updates the global default trial length.
//
//	@Summary	Update platform settings
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		updateSettingsRequest	true	"Settings"
//	@Success	200		{object}	appSettingsDTO
//	@Router		/admin/settings [patch]
func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req updateSettingsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	if req.DefaultTrialDays == nil || *req.DefaultTrialDays < 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "default_trial_days must be >= 0")
		return
	}
	settings, err := s.billing.UpdateTrialDays(r.Context(), *req.DefaultTrialDays)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not update settings")
		return
	}
	respond(w, http.StatusOK, settings)
}

// handleAdminListOrgs lists all orgs with subscriptions.
//
//	@Summary	List all organizations (admin)
//	@Tags		admin
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{array}	billing.AdminOrgItem
//	@Router		/admin/orgs [get]
func (s *Server) handleAdminListOrgs(w http.ResponseWriter, r *http.Request) {
	items, err := s.billing.AdminList(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not list organizations")
		return
	}
	respond(w, http.StatusOK, items)
}

type updateSubscriptionRequest struct {
	BillingEnabled    *bool   `json:"billing_enabled"`
	Status            *string `json:"status"`
	TrialDays         *int    `json:"trial_days"`
	PricePerSeatCents *int    `json:"price_per_seat_cents"`
	Currency          *string `json:"currency"`
}

// handleAdminUpdateSubscription updates an org's subscription (platform admin).
//
//	@Summary	Update an org subscription (admin)
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		orgID	path		string						true	"Organization ID"
//	@Param		body	body		updateSubscriptionRequest	true	"Subscription changes"
//	@Success	200		{object}	billing.View
//	@Router		/admin/orgs/{orgID}/subscription [patch]
func (s *Server) handleAdminUpdateSubscription(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")

	var req updateSubscriptionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	if req.TrialDays != nil && *req.TrialDays < 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "trial_days must be >= 0")
		return
	}
	if req.PricePerSeatCents != nil && *req.PricePerSeatCents < 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "price_per_seat_cents must be >= 0")
		return
	}

	view, err := s.billing.AdminUpdate(r.Context(), orgID, billing.AdminUpdateParams{
		BillingEnabled:    req.BillingEnabled,
		Status:            req.Status,
		TrialDays:         req.TrialDays,
		PricePerSeatCents: req.PricePerSeatCents,
		Currency:          req.Currency,
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "organization not found")
		case errors.Is(err, billing.ErrInvalidStatus):
			writeError(w, http.StatusBadRequest, "validation_error", "invalid status")
		case errors.Is(err, billing.ErrInvalidCurrency):
			writeError(w, http.StatusBadRequest, "validation_error", "unsupported currency")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "could not update subscription")
		}
		return
	}
	respond(w, http.StatusOK, view)
}
