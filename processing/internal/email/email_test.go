package email

import (
	"testing"
	"time"
)

func TestNewMailer_LogMailer(t *testing.T) {
	m, err := NewMailer(MailConfig{AppBaseURL: "http://localhost:3000"})
	if err != nil {
		t.Fatalf("NewMailer: %v", err)
	}
	if m.IsSMTP() {
		t.Error("expected LogMailer when no SMTP host")
	}
}

func TestNewMailer_SMTPMailer(t *testing.T) {
	m, err := NewMailer(MailConfig{
		SMTPHost:   "smtp.example.com",
		SMTPFrom:   "noreply@example.com",
		AppBaseURL: "http://localhost:3000",
	})
	if err != nil {
		t.Fatalf("NewMailer: %v", err)
	}
	if !m.IsSMTP() {
		t.Error("expected SMTPMailer when SMTP host is set")
	}
}

func TestLogMailer_SendVerification(t *testing.T) {
	m := &LogMailer{cfg: MailConfig{AppBaseURL: "http://app.test"}}
	link, err := m.SendVerification("user@test.com", "User", "tok123", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if link != "http://app.test/verify-email?token=tok123" {
		t.Errorf("link = %q", link)
	}
}

func TestLogMailer_SendPasswordReset(t *testing.T) {
	m := &LogMailer{cfg: MailConfig{AppBaseURL: "http://app.test"}}
	link, err := m.SendPasswordReset("user@test.com", "User", "reset123", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if link != "http://app.test/reset-password?token=reset123" {
		t.Errorf("link = %q", link)
	}
}

func TestLogMailer_SendInvite(t *testing.T) {
	m := &LogMailer{cfg: MailConfig{AppBaseURL: "http://app.test"}}
	link, err := m.SendInvite("new@test.com", "MyOrg", "Alice", "inv123", "member", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if link != "http://app.test/auth/invite?token=inv123" {
		t.Errorf("link = %q", link)
	}
}

func TestLogMailer_SendDeletionNotice(t *testing.T) {
	m := &LogMailer{cfg: MailConfig{AppBaseURL: "http://app.test"}}
	err := m.SendDeletionNotice("user@test.com", "User", "MyOrg", "en", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLogMailer_SendDeletionWarning(t *testing.T) {
	m := &LogMailer{cfg: MailConfig{AppBaseURL: "http://app.test"}}
	err := m.SendDeletionWarning("user@test.com", "User", "MyOrg", "en", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLogMailer_SendAlert(t *testing.T) {
	m := &LogMailer{cfg: MailConfig{AppBaseURL: "http://app.test"}}
	err := m.SendAlert("user@test.com", "User", "High Latency", "avg_latency", "500ms", "200ms", "http://app.test/dashboard", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLogMailer_IsSMTP(t *testing.T) {
	m := &LogMailer{}
	if m.IsSMTP() {
		t.Error("LogMailer.IsSMTP() should return false")
	}
}

func TestSMTPMailer_IsSMTP(t *testing.T) {
	m := &SMTPMailer{}
	if !m.IsSMTP() {
		t.Error("SMTPMailer.IsSMTP() should return true")
	}
}

func TestSanitizeHeader(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"normal header", "normal header"},
		{"has\rnewline", "hasnewline"},
		{"has\nnewline", "hasnewline"},
		{"has\r\nboth", "hasboth"},
	}
	for _, tc := range tests {
		got := sanitizeHeader(tc.input)
		if got != tc.want {
			t.Errorf("sanitizeHeader(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
