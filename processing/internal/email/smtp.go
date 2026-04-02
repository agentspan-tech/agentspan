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
	link := fmt.Sprintf("%s/auth/verify?token=%s", m.cfg.AppBaseURL, token)
	textBody := fmt.Sprintf("Hi %s,\n\nVerify your email address:\n%s\n\nIf you did not sign up for AgentSpan, you can ignore this email.\n", name, link)
	data := verificationData{Name: name, Link: link}
	if err := m.sendHTML(to, "Verify your AgentSpan email", textBody, "verification.html", data); err != nil {
		return "", err
	}
	return link, nil
}

func (m *SMTPMailer) SendPasswordReset(to, name, token, locale string) (string, error) {
	link := fmt.Sprintf("%s/auth/reset-password?token=%s", m.cfg.AppBaseURL, token)
	textBody := fmt.Sprintf("Hi %s,\n\nReset your AgentSpan password:\n%s\n\nThis link expires in 1 hour. If you did not request a password reset, you can ignore this email.\n", name, link)
	data := passwordResetData{Name: name, Link: link}
	if err := m.sendHTML(to, "Reset your AgentSpan password", textBody, "password_reset.html", data); err != nil {
		return "", err
	}
	return link, nil
}

func (m *SMTPMailer) SendInvite(to, orgName, inviterName, token, role, locale string) (string, error) {
	link := fmt.Sprintf("%s/auth/invite?token=%s", m.cfg.AppBaseURL, token)
	textBody := fmt.Sprintf("Hi,\n\n%s has invited you to join %s as %s.\n\nAccept your invite:\n%s\n\nThis link expires in 7 days.\n", inviterName, orgName, role, link)
	data := inviteData{InviterName: inviterName, OrgName: orgName, Role: role, Link: link}
	if err := m.sendHTML(to, fmt.Sprintf("You've been invited to join %s on AgentSpan", orgName), textBody, "invite.html", data); err != nil {
		return "", err
	}
	return link, nil
}

func (m *SMTPMailer) SendDeletionNotice(to, name, orgName, locale string, scheduledAt time.Time) error {
	textBody := fmt.Sprintf("Hi %s,\n\nYour organization %q has been scheduled for deletion on %s.\n\nYou have 14 days to cancel this action from the organization settings.\n", name, orgName, scheduledAt.Format("January 2, 2006"))
	data := deletionNoticeData{Name: name, OrgName: orgName, ScheduledAt: scheduledAt.Format("January 2, 2006")}
	return m.sendHTML(to, fmt.Sprintf("Your organization %q is scheduled for deletion", orgName), textBody, "deletion_notice.html", data)
}

func (m *SMTPMailer) SendDeletionWarning(to, name, orgName, locale string, deletionAt time.Time) error {
	textBody := fmt.Sprintf("Hi %s,\n\nThis is a reminder that your organization %q will be permanently deleted on %s.\n\nTo cancel, visit your organization settings before the deletion date.\n", name, orgName, deletionAt.Format("January 2, 2006"))
	data := deletionWarningData{Name: name, OrgName: orgName, DeletionAt: deletionAt.Format("January 2, 2006")}
	return m.sendHTML(to, fmt.Sprintf("Reminder: %q will be deleted soon", orgName), textBody, "deletion_warning.html", data)
}

func (m *SMTPMailer) SendAlert(to, name, alertName, alertType, currentValue, threshold, dashboardLink, locale string) error {
	subject := fmt.Sprintf("[AgentSpan] Alert: %s", alertName)
	textBody := fmt.Sprintf("Hi %s,\n\nAlert \"%s\" has triggered.\n\nType: %s\nCurrent value: %s\nThreshold: %s\n\nView dashboard: %s\n", name, alertName, alertType, currentValue, threshold, dashboardLink)
	data := alertData{Name: name, AlertName: alertName, AlertType: alertType, CurrentValue: currentValue, Threshold: threshold, DashboardLink: dashboardLink}
	return m.sendHTML(to, subject, textBody, "alert.html", data)
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

	boundary := fmt.Sprintf("agentspan-%d", time.Now().UnixNano())
	headers := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%s\r\n\r\n",
		sanitizeHeader(m.cfg.SMTPFrom), sanitizeHeader(to), sanitizeHeader(subject), boundary,
	)
	body := fmt.Sprintf(
		"--%s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n--%s\r\nContent-Type: text/html; charset=utf-8\r\n\r\n%s\r\n--%s--",
		boundary, textBody, boundary, htmlBuf.String(), boundary,
	)
	msg := headers + body
	addr := fmt.Sprintf("%s:%d", m.cfg.SMTPHost, m.cfg.SMTPPort)
	auth := smtp.PlainAuth("", m.cfg.SMTPUser, m.cfg.SMTPPass, m.cfg.SMTPHost)
	return smtp.SendMail(addr, auth, m.cfg.SMTPFrom, []string{to}, []byte(msg))
}

func (m *SMTPMailer) send(to, subject, body string) error {
	addr := fmt.Sprintf("%s:%d", m.cfg.SMTPHost, m.cfg.SMTPPort)
	auth := smtp.PlainAuth("", m.cfg.SMTPUser, m.cfg.SMTPPass, m.cfg.SMTPHost)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s", sanitizeHeader(m.cfg.SMTPFrom), sanitizeHeader(to), sanitizeHeader(subject), body)
	if err := smtp.SendMail(addr, auth, m.cfg.SMTPFrom, []string{to}, []byte(msg)); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}
	return nil
}
