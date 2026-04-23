package email

import (
	"fmt"
	"net/mail"
	"time"
)

// MailConfig holds SMTP and application configuration for the mailer.
//
// SMTPFrom is the operator-provided value (bare address or RFC 5322 name-addr).
// SMTPFromEnvelope and SMTPFromHeader are populated by NewMailer from SMTPFrom
// and must not be set by callers: the envelope form is used for SMTP MAIL FROM
// (RFC 5321 — bare address only), while the header form is used for the
// message From: header (RFC 5322 — display-name allowed).
type MailConfig struct {
	SMTPHost         string
	SMTPPort         int
	SMTPUser         string
	SMTPPass         string
	SMTPFrom         string
	SMTPFromEnvelope string
	SMTPFromHeader   string
	AppBaseURL       string
}

// Mailer is the interface for sending transactional emails.
// All Send* methods accept a locale parameter for future multi-language support (MAIL-03).
type Mailer interface {
	// SendVerification sends an email verification link.
	// Returns the verification link (used by handlers when SMTP is disabled).
	SendVerification(to, name, token, locale string) (link string, err error)

	// SendPasswordReset sends a password reset link.
	// Returns the reset link (used by handlers when SMTP is disabled).
	SendPasswordReset(to, name, token, locale string) (link string, err error)

	// SendInvite sends an organization invite link.
	// Returns the invite link (used by handlers when SMTP is disabled).
	SendInvite(to, orgName, inviterName, token, role, locale string) (link string, err error)

	// SendDeletionNotice sends a notice that the organization has been scheduled for deletion.
	SendDeletionNotice(to, name, orgName, locale string, scheduledAt time.Time) error

	// SendDeletionWarning sends a warning that deletion is imminent.
	SendDeletionWarning(to, name, orgName, locale string, deletionAt time.Time) error

	// SendAlert sends an alert notification email.
	// Returns error only — no link needed (D-08).
	SendAlert(to, name, alertName, alertType, currentValue, threshold, dashboardLink, locale string) error

	// IsSMTP returns true when SMTP delivery is active.
	// Handlers use this to decide whether to include links in API responses (D-07, D-08).
	IsSMTP() bool
}

// NewMailer returns an SMTPMailer when SMTPHost is configured, or a LogMailer otherwise.
//
// When SMTPHost is set, SMTPFrom must be a valid RFC 5322 address (bare or
// with display-name). NewMailer parses it once and populates SMTPFromEnvelope
// (bare address for SMTP MAIL FROM) and SMTPFromHeader (original string for
// the message From: header). An invalid SMTPFrom returns an error so callers
// can fail-fast at startup.
func NewMailer(cfg MailConfig) (Mailer, error) {
	if cfg.SMTPHost == "" {
		return &LogMailer{cfg: cfg}, nil
	}
	if cfg.SMTPFrom == "" {
		return nil, fmt.Errorf("SMTP_FROM is required when SMTP_HOST is set")
	}
	addr, err := mail.ParseAddress(cfg.SMTPFrom)
	if err != nil {
		return nil, fmt.Errorf("parse SMTP_FROM %q: %w", cfg.SMTPFrom, err)
	}
	cfg.SMTPFromEnvelope = addr.Address
	cfg.SMTPFromHeader = cfg.SMTPFrom
	return &SMTPMailer{cfg: cfg}, nil
}
