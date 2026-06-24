package http

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"cairn/internal/auth"
)

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

type resetPasswordRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// handleForgotPassword emails a reset link for the given address. It always
// responds 200 with the same message whether or not the email is registered,
// so the endpoint can't be used to enumerate accounts.
//
//	@Summary	Request a password reset
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		forgotPasswordRequest	true	"Email to send a reset link to"
//	@Success	200		{object}	successEnvelope
//	@Failure	400		{object}	errorEnvelope
//	@Router		/auth/forgot-password [post]
func (s *Server) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotPasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	if strings.TrimSpace(req.Email) == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "email is required")
		return
	}

	rawToken, user, err := s.auth.RequestPasswordReset(r.Context(), req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not process request")
		return
	}
	if user != nil {
		resetURL := s.cfg.FrontendURL + "/reset-password?token=" + url.QueryEscape(rawToken)
		if err := s.mailer.SendPasswordReset(user.Email, resetURL); err != nil {
			// Delivery failures must not reveal whether the account exists; log only.
			slog.Error("send password reset email", "error", err)
		}
	}

	respondMsg(w, http.StatusOK, "If an account exists for that email, a reset link is on its way.", nil)
}

// handleResetPassword consumes a reset token and sets a new password.
//
//	@Summary	Reset password with a token
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		resetPasswordRequest	true	"Reset token and new password"
//	@Success	200		{object}	successEnvelope
//	@Failure	400		{object}	errorEnvelope
//	@Router		/auth/reset-password [post]
func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not parse request body")
		return
	}
	if strings.TrimSpace(req.Token) == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "token is required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "validation_error", "password must be at least 8 characters")
		return
	}

	if err := s.auth.ResetPassword(r.Context(), req.Token, req.Password); err != nil {
		if errors.Is(err, auth.ErrInvalidToken) {
			writeError(w, http.StatusBadRequest, "invalid_token", "this reset link is invalid or has expired")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "could not reset password")
		return
	}

	respondMsg(w, http.StatusOK, "Your password has been reset. You can now log in.", nil)
}
