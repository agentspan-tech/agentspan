package email

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"net/smtp"
	"strings"
	"time"
)

//go:embed templates/*.html
var templateFS embed.FS

var emailTemplates *template.Template

func init() {
	emailTemplates = template.Must(template.ParseFS(templateFS, "templates/*.html"))
}

// Template data structs

type verificationData struct {
	Name string
	Link string
}

type passwordResetData struct {
	Name string
	Link string
}

type inviteData struct {
	InviterName string
	OrgName     string
	Role        string
	Link        string
}

type deletionNoticeData struct {
	Name        string
	OrgName     string
	ScheduledAt string
}

type deletionWarningData struct {
	Name       string
	OrgName    string
	DeletionAt string
}

type alertData struct {
	Name          string
	AlertName     string
	AlertType     string
	CurrentValue  string
	Threshold     string
	DashboardLink string
}

// SMTPMailer sends emails via SMTP using net/smtp with PlainAuth.
type SMTPMailer struct {
	cfg MailConfig
}

func (m *SMTPMailer) IsSMTP() bool { return true }

func (m *SMTPMailer) SendVerification(to, name, token, locale string) (string, error) {
	link := fmt.Sprintf("%s/verify-email?token=%s", m.cfg.AppBaseURL, token)
	textBody := textVerification(name, link, locale)
	data := verificationData{Name: name, Link: link}
	if err := m.sendHTML(to, subject("verification", locale), textBody, templateName("verification", locale), data); err != nil {
		return "", err
	}
	return link, nil
}

func (m *SMTPMailer) SendPasswordReset(to, name, token, locale string) (string, error) {
	link := fmt.Sprintf("%s/reset-password?token=%s", m.cfg.AppBaseURL, token)
	textBody := textPasswordReset(name, link, locale)
	data := passwordResetData{Name: name, Link: link}
	if err := m.sendHTML(to, subject("password_reset", locale), textBody, templateName("password_reset", locale), data); err != nil {
		return "", err
	}
	return link, nil
}

func (m *SMTPMailer) SendInvite(to, orgName, inviterName, token, role, locale string) (string, error) {
	link := fmt.Sprintf("%s/auth/invite?token=%s", m.cfg.AppBaseURL, token)
	textBody := textInvite(inviterName, orgName, role, link, locale)
	data := inviteData{InviterName: inviterName, OrgName: orgName, Role: role, Link: link}
	if err := m.sendHTML(to, fmt.Sprintf(subject("invite", locale), orgName), textBody, templateName("invite", locale), data); err != nil {
		return "", err
	}
	return link, nil
}

func (m *SMTPMailer) SendDeletionNotice(to, name, orgName, locale string, scheduledAt time.Time) error {
	scheduledStr := formatDate(scheduledAt, locale)
	textBody := textDeletionNotice(name, orgName, locale, scheduledAt)
	data := deletionNoticeData{Name: name, OrgName: orgName, ScheduledAt: scheduledStr}
	return m.sendHTML(to, fmt.Sprintf(subject("deletion_notice", locale), orgName), textBody, templateName("deletion_notice", locale), data)
}

func (m *SMTPMailer) SendDeletionWarning(to, name, orgName, locale string, deletionAt time.Time) error {
	deletionStr := formatDate(deletionAt, locale)
	textBody := textDeletionWarning(name, orgName, locale, deletionAt)
	data := deletionWarningData{Name: name, OrgName: orgName, DeletionAt: deletionStr}
	return m.sendHTML(to, fmt.Sprintf(subject("deletion_warning", locale), orgName), textBody, templateName("deletion_warning", locale), data)
}

func (m *SMTPMailer) SendAlert(to, name, alertName, alertType, currentValue, threshold, dashboardLink, locale string) error {
	textBody := textAlert(name, alertName, alertType, currentValue, threshold, dashboardLink, locale)
	data := alertData{Name: name, AlertName: alertName, AlertType: alertType, CurrentValue: currentValue, Threshold: threshold, DashboardLink: dashboardLink}
	return m.sendHTML(to, fmt.Sprintf(subject("alert", locale), alertName), textBody, templateName("alert", locale), data)
}

// formatDate renders a date in a locale-appropriate long form.
func formatDate(t time.Time, locale string) string {
	if normalizeLocale(locale) == "ru" {
		return t.Format("2 January 2006")
	}
	return t.Format("January 2, 2006")
}

// sanitizeHeader strips CR/LF characters from a header value to prevent header injection.
func sanitizeHeader(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

// sendHTML sends an email with multipart/alternative containing both plain text and HTML parts.
// Falls back to plain text if template rendering fails.
func (m *SMTPMailer) sendHTML(to, subject, textBody string, tmplName string, data interface{}) error {
	var htmlBuf bytes.Buffer
	if err := emailTemplates.ExecuteTemplate(&htmlBuf, tmplName, data); err != nil {
		// Fallback to plain text if template fails
		return m.send(to, subject, textBody)
	}

	boundary := fmt.Sprintf("agentorbit-%d", time.Now().UnixNano())
	headers := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%s\r\n\r\n",
		sanitizeHeader(m.cfg.SMTPFromHeader), sanitizeHeader(to), sanitizeHeader(subject), boundary,
	)
	body := fmt.Sprintf(
		"--%s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n--%s\r\nContent-Type: text/html; charset=utf-8\r\n\r\n%s\r\n--%s--",
		boundary, textBody, boundary, htmlBuf.String(), boundary,
	)
	msg := headers + body
	addr := fmt.Sprintf("%s:%d", m.cfg.SMTPHost, m.cfg.SMTPPort)
	auth := smtp.PlainAuth("", m.cfg.SMTPUser, m.cfg.SMTPPass, m.cfg.SMTPHost)
	return smtp.SendMail(addr, auth, m.cfg.SMTPFromEnvelope, []string{to}, []byte(msg))
}

func (m *SMTPMailer) send(to, subject, body string) error {
	addr := fmt.Sprintf("%s:%d", m.cfg.SMTPHost, m.cfg.SMTPPort)
	auth := smtp.PlainAuth("", m.cfg.SMTPUser, m.cfg.SMTPPass, m.cfg.SMTPHost)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s", sanitizeHeader(m.cfg.SMTPFromHeader), sanitizeHeader(to), sanitizeHeader(subject), body)
	if err := smtp.SendMail(addr, auth, m.cfg.SMTPFromEnvelope, []string{to}, []byte(msg)); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}
	return nil
}
