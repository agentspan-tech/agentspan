package email

import (
	"fmt"
	"log/slog"
	"time"
)

// LogMailer logs email actions without sending actual emails.
// Used when SMTP is not configured (development, testing, or self-host with copyable links).
//
// SECURITY: LogMailer MUST NOT log tokens, links, or any credential material.
// Only the action type and recipient address are logged. Links are returned
// to the caller for inclusion in API responses (D-07) but never written to logs.
type LogMailer struct {
	cfg MailConfig
}

func (m *LogMailer) IsSMTP() bool { return false }

func (m *LogMailer) SendVerification(to, name, token, locale string) (string, error) {
	slog.Info("email (no SMTP)", "type", "verification", "to", to)
	link := fmt.Sprintf("%s/auth/verify?token=%s", m.cfg.AppBaseURL, token)
	return link, nil
}

func (m *LogMailer) SendPasswordReset(to, name, token, locale string) (string, error) {
	slog.Info("email (no SMTP)", "type", "password_reset", "to", to)
	link := fmt.Sprintf("%s/auth/reset-password?token=%s", m.cfg.AppBaseURL, token)
	return link, nil
}

func (m *LogMailer) SendInvite(to, orgName, inviterName, token, role, locale string) (string, error) {
	slog.Info("email (no SMTP)", "type", "invite", "to", to)
	link := fmt.Sprintf("%s/auth/invite?token=%s", m.cfg.AppBaseURL, token)
	return link, nil
}

func (m *LogMailer) SendDeletionNotice(to, name, orgName, locale string, scheduledAt time.Time) error {
	slog.Info("email (no SMTP)", "type", "deletion_notice", "to", to)
	return nil
}

func (m *LogMailer) SendDeletionWarning(to, name, orgName, locale string, deletionAt time.Time) error {
	slog.Info("email (no SMTP)", "type", "deletion_warning", "to", to)
	return nil
}

func (m *LogMailer) SendAlert(to, name, alertName, alertType, currentValue, threshold, dashboardLink, locale string) error {
	slog.Info("email (no SMTP)", "type", "alert", "to", to, "alert", alertName)
	return nil
}
