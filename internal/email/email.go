// Package email sends transactional mail (e.g. organization invitations).
// When SMTP is not configured it logs messages instead of sending, so flows
// remain testable in local development.
package email

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strconv"
	"strings"

	"cairn/internal/config"
)

// Sender delivers email over SMTP.
type Sender struct {
	cfg     config.SMTPConfig
	enabled bool
}

// New builds a Sender. It is "enabled" only when host and credentials are present.
func New(cfg config.SMTPConfig) *Sender {
	enabled := cfg.Host != "" && cfg.User != "" && cfg.Password != ""
	if !enabled {
		slog.Warn("smtp not fully configured; emails will be logged instead of sent")
	}
	return &Sender{cfg: cfg, enabled: enabled}
}

// SendInvitation sends an organization invitation email.
func (s *Sender) SendInvitation(to, orgName, inviteURL string) error {
	subject := fmt.Sprintf("You've been invited to join %s on Cairn", orgName)
	body := fmt.Sprintf(
		`<p>You've been invited to join <strong>%s</strong> on Cairn.</p>`+
			`<p><a href="%s">Accept the invitation</a></p>`+
			`<p>If the link doesn't work, paste this URL into your browser:<br>%s</p>`,
		orgName, inviteURL, inviteURL,
	)
	return s.send(to, subject, body)
}

// SendPasswordReset sends a password-reset link.
func (s *Sender) SendPasswordReset(to, resetURL string) error {
	subject := "Reset your Cairn password"
	body := fmt.Sprintf(
		`<p>We received a request to reset your Cairn password.</p>`+
			`<p><a href="%s">Choose a new password</a></p>`+
			`<p>If the link doesn't work, paste this URL into your browser:<br>%s</p>`+
			`<p>This link expires in 1 hour. If you didn't request a reset, you can safely ignore this email.</p>`,
		resetURL, resetURL,
	)
	return s.send(to, subject, body)
}

func (s *Sender) send(to, subject, htmlBody string) error {
	from := s.cfg.From
	if from == "" {
		from = s.cfg.User
	}

	if !s.enabled {
		slog.Info("email (smtp disabled): not sent", "to", to, "subject", subject)
		return nil
	}

	msg := buildMessage(from, to, subject, htmlBody)
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	auth := smtp.PlainAuth("", s.cfg.User, s.cfg.Password, s.cfg.Host)

	// Port 465 uses implicit TLS (SMTPS); other ports use STARTTLS via SendMail.
	if s.cfg.Port == 465 {
		return s.sendImplicitTLS(addr, from, to, auth, msg)
	}
	if err := smtp.SendMail(addr, auth, from, []string{to}, msg); err != nil {
		return fmt.Errorf("email: send: %w", err)
	}
	return nil
}

func (s *Sender) sendImplicitTLS(addr, from, to string, auth smtp.Auth, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: s.cfg.Host})
	if err != nil {
		return fmt.Errorf("email: tls dial: %w", err)
	}
	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return fmt.Errorf("email: smtp client: %w", err)
	}
	defer client.Close()

	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("email: auth: %w", err)
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("email: mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("email: rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("email: data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("email: write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("email: close data: %w", err)
	}
	return client.Quit()
}

func buildMessage(from, to, subject, htmlBody string) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(htmlBody)
	return []byte(b.String())
}
