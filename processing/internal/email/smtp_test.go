package email

import (
	"bufio"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSmtpServer accepts a single SMTP conversation and captures the message body.
type fakeSmtpServer struct {
	addr    string
	message string
	from    string
	to      string
	mu      sync.Mutex
	done    chan struct{}
}

func newFakeSMTP(t *testing.T) *fakeSmtpServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &fakeSmtpServer{
		addr: ln.Addr().String(),
		done: make(chan struct{}),
	}
	go func() {
		defer close(s.done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		defer ln.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck

		w := bufio.NewWriter(conn)
		r := bufio.NewReader(conn)

		write := func(line string) {
			w.WriteString(line + "\r\n") //nolint:errcheck
			w.Flush()                    //nolint:errcheck
		}

		write("220 localhost SMTP ready")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSpace(line)
			cmd := strings.ToUpper(line)

			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				write("250-localhost")
				write("250 AUTH PLAIN")
			case strings.HasPrefix(cmd, "AUTH"):
				write("235 OK")
			case strings.HasPrefix(cmd, "MAIL FROM:"):
				s.mu.Lock()
				s.from = line
				s.mu.Unlock()
				write("250 OK")
			case strings.HasPrefix(cmd, "RCPT TO:"):
				s.mu.Lock()
				s.to = line
				s.mu.Unlock()
				write("250 OK")
			case strings.HasPrefix(cmd, "DATA"):
				write("354 Go ahead")
				var body strings.Builder
				for {
					dataLine, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if strings.TrimSpace(dataLine) == "." {
						break
					}
					body.WriteString(dataLine)
				}
				s.mu.Lock()
				s.message = body.String()
				s.mu.Unlock()
				write("250 OK")
			case strings.HasPrefix(cmd, "QUIT"):
				write("221 Bye")
				return
			default:
				write("500 Unknown command")
			}
		}
	}()
	return s
}

func (s *fakeSmtpServer) wait(t *testing.T) {
	t.Helper()
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
		t.Fatal("fake SMTP server timed out")
	}
}

func newTestMailer(addr string) *SMTPMailer {
	return newTestMailerFrom(addr, "noreply@agentorbit.dev")
}

func newTestMailerFrom(addr, from string) *SMTPMailer {
	host, port, _ := net.SplitHostPort(addr)
	p := 0
	for _, c := range port {
		p = p*10 + int(c-'0')
	}
	m, err := NewMailer(MailConfig{
		SMTPHost:   host,
		SMTPPort:   p,
		SMTPUser:   "user",
		SMTPPass:   "pass",
		SMTPFrom:   from,
		AppBaseURL: "https://app.agentorbit.dev",
	})
	if err != nil {
		panic(err)
	}
	return m.(*SMTPMailer)
}

func TestSMTPMailer_SendVerification(t *testing.T) {
	srv := newFakeSMTP(t)
	m := newTestMailer(srv.addr)

	link, err := m.SendVerification("user@test.com", "Alice", "tok123", "en")
	srv.wait(t)

	if err != nil {
		t.Fatalf("SendVerification: %v", err)
	}
	if link != "https://app.agentorbit.dev/verify-email?token=tok123" {
		t.Errorf("link = %q", link)
	}
	if !strings.Contains(srv.message, "Verify your email") {
		t.Error("expected HTML template content in message")
	}
	if !strings.Contains(srv.message, "Alice") {
		t.Error("expected name in message")
	}
	if !strings.Contains(srv.message, "multipart/alternative") {
		t.Error("expected multipart content type")
	}
}

func TestSMTPMailer_SendPasswordReset(t *testing.T) {
	srv := newFakeSMTP(t)
	m := newTestMailer(srv.addr)

	link, err := m.SendPasswordReset("user@test.com", "Bob", "reset456", "en")
	srv.wait(t)

	if err != nil {
		t.Fatalf("SendPasswordReset: %v", err)
	}
	if link != "https://app.agentorbit.dev/reset-password?token=reset456" {
		t.Errorf("link = %q", link)
	}
	if !strings.Contains(srv.message, "Bob") {
		t.Error("expected name in message")
	}
}

func TestSMTPMailer_SendInvite(t *testing.T) {
	srv := newFakeSMTP(t)
	m := newTestMailer(srv.addr)

	link, err := m.SendInvite("new@test.com", "TestOrg", "Alice", "inv789", "admin", "en")
	srv.wait(t)

	if err != nil {
		t.Fatalf("SendInvite: %v", err)
	}
	if link != "https://app.agentorbit.dev/auth/invite?token=inv789" {
		t.Errorf("link = %q", link)
	}
	if !strings.Contains(srv.message, "TestOrg") {
		t.Error("expected org name in message")
	}
}

func TestSMTPMailer_SendDeletionNotice(t *testing.T) {
	srv := newFakeSMTP(t)
	m := newTestMailer(srv.addr)

	ts := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	err := m.SendDeletionNotice("user@test.com", "Alice", "TestOrg", "en", ts)
	srv.wait(t)

	if err != nil {
		t.Fatalf("SendDeletionNotice: %v", err)
	}
	if !strings.Contains(srv.message, "April 15, 2026") {
		t.Error("expected formatted date in message")
	}
}

func TestSMTPMailer_SendDeletionWarning(t *testing.T) {
	srv := newFakeSMTP(t)
	m := newTestMailer(srv.addr)

	ts := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	err := m.SendDeletionWarning("user@test.com", "Alice", "TestOrg", "en", ts)
	srv.wait(t)

	if err != nil {
		t.Fatalf("SendDeletionWarning: %v", err)
	}
	if !strings.Contains(srv.message, "April 20, 2026") {
		t.Error("expected formatted date in message")
	}
}

func TestSMTPMailer_SendAlert(t *testing.T) {
	srv := newFakeSMTP(t)
	m := newTestMailer(srv.addr)

	err := m.SendAlert("user@test.com", "Alice", "High Latency", "avg_latency", "500ms", "200ms", "https://app.agentorbit.dev/dashboard", "en")
	srv.wait(t)

	if err != nil {
		t.Fatalf("SendAlert: %v", err)
	}
	if !strings.Contains(srv.message, "High Latency") {
		t.Error("expected alert name in message")
	}
	if !strings.Contains(srv.message, "500ms") {
		t.Error("expected current value in message")
	}
}

func TestSMTPMailer_MessageFormat(t *testing.T) {
	srv := newFakeSMTP(t)
	m := newTestMailer(srv.addr)

	_, err := m.SendVerification("user@test.com", "Test", "tok", "en")
	srv.wait(t)

	if err != nil {
		t.Fatalf("SendVerification: %v", err)
	}

	// Verify MIME structure.
	if !strings.Contains(srv.message, "MIME-Version: 1.0") {
		t.Error("expected MIME-Version header")
	}
	if !strings.Contains(srv.message, "text/plain") {
		t.Error("expected text/plain part")
	}
	if !strings.Contains(srv.message, "text/html") {
		t.Error("expected text/html part")
	}
	if !strings.Contains(srv.message, "From: noreply@agentorbit.dev") {
		t.Error("expected From header")
	}
}

// TestNewMailer_ParsesSMTPFrom verifies that NewMailer splits SMTP_FROM into
// envelope (bare address) and header (full string) forms.
func TestNewMailer_ParsesSMTPFrom(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantEnvelope string
		wantHeader   string
	}{
		{
			name:         "bare address",
			input:        "noreply@agentorbit.tech",
			wantEnvelope: "noreply@agentorbit.tech",
			wantHeader:   "noreply@agentorbit.tech",
		},
		{
			name:         "display name",
			input:        "AgentOrbit <noreply@agentorbit.tech>",
			wantEnvelope: "noreply@agentorbit.tech",
			wantHeader:   "AgentOrbit <noreply@agentorbit.tech>",
		},
		{
			name:         "quoted display name with comma",
			input:        `"Agent, Span" <noreply@agentorbit.tech>`,
			wantEnvelope: "noreply@agentorbit.tech",
			wantHeader:   `"Agent, Span" <noreply@agentorbit.tech>`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := NewMailer(MailConfig{
				SMTPHost:   "smtp.example.com",
				SMTPPort:   587,
				SMTPUser:   "u",
				SMTPPass:   "p",
				SMTPFrom:   tc.input,
				AppBaseURL: "https://example.com",
			})
			if err != nil {
				t.Fatalf("NewMailer: %v", err)
			}
			sm, ok := m.(*SMTPMailer)
			if !ok {
				t.Fatalf("expected *SMTPMailer, got %T", m)
			}
			if sm.cfg.SMTPFromEnvelope != tc.wantEnvelope {
				t.Errorf("envelope = %q, want %q", sm.cfg.SMTPFromEnvelope, tc.wantEnvelope)
			}
			if sm.cfg.SMTPFromHeader != tc.wantHeader {
				t.Errorf("header = %q, want %q", sm.cfg.SMTPFromHeader, tc.wantHeader)
			}
		})
	}
}

// TestNewMailer_RejectsInvalidSMTPFrom verifies fail-fast on unparseable SMTP_FROM
// when SMTPHost is configured.
func TestNewMailer_RejectsInvalidSMTPFrom(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"garbage", "not-an-email"},
		{"unterminated angle", "AgentOrbit <noreply@agentorbit.tech"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewMailer(MailConfig{
				SMTPHost:   "smtp.example.com",
				SMTPPort:   587,
				SMTPFrom:   tc.input,
				AppBaseURL: "https://example.com",
			})
			if err == nil {
				t.Fatal("expected error for invalid SMTP_FROM, got nil")
			}
		})
	}
}

// TestNewMailer_AllowsEmptySMTPFromWithoutHost verifies that log-only mode does
// not require SMTP_FROM.
func TestNewMailer_AllowsEmptySMTPFromWithoutHost(t *testing.T) {
	m, err := NewMailer(MailConfig{AppBaseURL: "https://example.com"})
	if err != nil {
		t.Fatalf("NewMailer: %v", err)
	}
	if m.IsSMTP() {
		t.Error("expected non-SMTP mailer when SMTPHost is empty")
	}
}

// TestSMTPMailer_EnvelopeUsesBareAddress verifies the bug fix: when SMTP_FROM
// contains a display-name, MAIL FROM envelope must contain only the bare address
// while the From: header contains the full display-name form.
func TestSMTPMailer_EnvelopeUsesBareAddress(t *testing.T) {
	srv := newFakeSMTP(t)
	m := newTestMailerFrom(srv.addr, "AgentOrbit <noreply@agentorbit.tech>")

	_, err := m.SendVerification("user@test.com", "Alice", "tok", "en")
	srv.wait(t)

	if err != nil {
		t.Fatalf("SendVerification: %v", err)
	}

	// MAIL FROM envelope must use bare address only (RFC 5321).
	if !strings.Contains(srv.from, "<noreply@agentorbit.tech>") {
		t.Errorf("MAIL FROM = %q, want envelope with bare address", srv.from)
	}
	if strings.Contains(srv.from, "AgentOrbit") {
		t.Errorf("MAIL FROM = %q must not contain display-name", srv.from)
	}

	// From: header in message body keeps full display-name form.
	if !strings.Contains(srv.message, "From: AgentOrbit <noreply@agentorbit.tech>") {
		t.Errorf("expected From: header with display-name in message body, got:\n%s", srv.message)
	}
}

// TestSMTPMailer_SubjectRFC2047Encoded verifies that non-ASCII Subject values
// are RFC 2047 encoded. Plain UTF-8 bytes in a Subject header violate RFC 5322
// section 2.2 and are rejected by some SMTP-over-HTTP providers (e.g. Resend)
// with a 500 error, which previously broke Russian-locale registration.
func TestSMTPMailer_SubjectRFC2047Encoded(t *testing.T) {
	srv := newFakeSMTP(t)
	m := newTestMailer(srv.addr)

	_, err := m.SendVerification("user@test.com", "Алиса", "tok", "ru")
	srv.wait(t)

	if err != nil {
		t.Fatalf("SendVerification: %v", err)
	}

	// Raw Cyrillic Subject must not leak into the header.
	if strings.Contains(srv.message, "Subject: Подтвердите") {
		t.Errorf("Subject header contains raw UTF-8 bytes:\n%s", srv.message)
	}
	// Encoded form should be present (QEncoding produces =?UTF-8?q?...?= or =?utf-8?q?...?=).
	if !strings.Contains(strings.ToLower(srv.message), "subject: =?utf-8?q?") {
		t.Errorf("expected RFC 2047 encoded Subject, got:\n%s", srv.message)
	}
}

// TestSMTPMailer_FromHeaderNonASCIIDisplayName verifies that non-ASCII
// display-names in From are RFC 2047 encoded while the email address stays bare.
func TestSMTPMailer_FromHeaderNonASCIIDisplayName(t *testing.T) {
	srv := newFakeSMTP(t)
	m := newTestMailerFrom(srv.addr, "АгентОрбит <noreply@agentorbit.tech>")

	_, err := m.SendVerification("user@test.com", "Alice", "tok", "en")
	srv.wait(t)

	if err != nil {
		t.Fatalf("SendVerification: %v", err)
	}

	if strings.Contains(srv.message, "From: АгентОрбит") {
		t.Errorf("From header contains raw UTF-8 display-name:\n%s", srv.message)
	}
	if !strings.Contains(strings.ToLower(srv.message), "from: =?utf-8?q?") {
		t.Errorf("expected RFC 2047 encoded From display-name, got:\n%s", srv.message)
	}
	if !strings.Contains(srv.message, "<noreply@agentorbit.tech>") {
		t.Errorf("expected bare address in From header, got:\n%s", srv.message)
	}
}

