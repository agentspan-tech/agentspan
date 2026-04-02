//go:build integration

package testutil

import "time"

// MockMailer implements email.Mailer for testing.
type MockMailer struct {
	Calls []string
}

func (m *MockMailer) IsSMTP() bool { return false }

func (m *MockMailer) SendVerification(to, name, token, locale string) (string, error) {
	m.Calls = append(m.Calls, "verification:"+to)
	return "http://test/verify?token=" + token, nil
}

func (m *MockMailer) SendPasswordReset(to, name, token, locale string) (string, error) {
	m.Calls = append(m.Calls, "password_reset:"+to)
	return "http://test/reset?token=" + token, nil
}

func (m *MockMailer) SendInvite(to, orgName, inviterName, token, role, locale string) (string, error) {
	m.Calls = append(m.Calls, "invite:"+to)
	return "http://test/invite?token=" + token, nil
}

func (m *MockMailer) SendDeletionNotice(to, name, orgName, locale string, scheduledAt time.Time) error {
	m.Calls = append(m.Calls, "deletion_notice:"+to)
	return nil
}

func (m *MockMailer) SendDeletionWarning(to, name, orgName, locale string, deletionAt time.Time) error {
	m.Calls = append(m.Calls, "deletion_warning:"+to)
	return nil
}

func (m *MockMailer) SendAlert(to, name, alertName, alertType, currentValue, threshold, dashboardLink, locale string) error {
	m.Calls = append(m.Calls, "alert:"+to)
	return nil
}
