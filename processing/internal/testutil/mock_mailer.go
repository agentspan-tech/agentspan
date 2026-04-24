//go:build integration

package testutil

import "time"

// MockMailer implements email.Mailer for testing.
//
// Calls is a log of "<kind>:<to>" entries (backward compatible). LastLocale
// captures the locale value passed to the most recent Send* call so tests can
// assert locale threading without inspecting the Calls strings.
type MockMailer struct {
	Calls      []string
	LastLocale string
	// VerificationErr, when set, is returned from SendVerification so tests can
	// exercise mailer-failure paths (e.g. transactional rollback on send failure).
	VerificationErr error
}

func (m *MockMailer) IsSMTP() bool { return false }

func (m *MockMailer) SendVerification(to, name, token, locale string) (string, error) {
	m.Calls = append(m.Calls, "verification:"+to)
	m.LastLocale = locale
	if m.VerificationErr != nil {
		return "", m.VerificationErr
	}
	return "http://test/verify?token=" + token, nil
}

func (m *MockMailer) SendPasswordReset(to, name, token, locale string) (string, error) {
	m.Calls = append(m.Calls, "password_reset:"+to)
	m.LastLocale = locale
	return "http://test/reset?token=" + token, nil
}

func (m *MockMailer) SendInvite(to, orgName, inviterName, token, role, locale string) (string, error) {
	m.Calls = append(m.Calls, "invite:"+to)
	m.LastLocale = locale
	return "http://test/invite?token=" + token, nil
}

func (m *MockMailer) SendDeletionNotice(to, name, orgName, locale string, scheduledAt time.Time) error {
	m.Calls = append(m.Calls, "deletion_notice:"+to)
	m.LastLocale = locale
	return nil
}

func (m *MockMailer) SendDeletionWarning(to, name, orgName, locale string, deletionAt time.Time) error {
	m.Calls = append(m.Calls, "deletion_warning:"+to)
	m.LastLocale = locale
	return nil
}

func (m *MockMailer) SendAlert(to, name, alertName, alertType, currentValue, threshold, dashboardLink, locale string) error {
	m.Calls = append(m.Calls, "alert:"+to)
	m.LastLocale = locale
	return nil
}
