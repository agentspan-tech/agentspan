package email

import "time"

// MailConfig holds SMTP and application configuration for the mailer.
type MailConfig struct {
	SMTPHost   string
	SMTPPort   int
	SMTPUser   string
	SMTPPass   string
	SMTPFrom   string
	AppBaseURL string
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
func NewMailer(cfg MailConfig) Mailer {
	if cfg.SMTPHost != "" {
		return &SMTPMailer{cfg: cfg}
	}
	return &LogMailer{cfg: cfg}
}
