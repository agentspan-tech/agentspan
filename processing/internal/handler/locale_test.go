package handler

import (
	"net/http/httptest"
	"testing"
)

func TestLocaleFromRequest(t *testing.T) {
	cases := []struct {
		header string
		want   string
	}{
		{"", "en"},
		{"en", "en"},
		{"en-US", "en"},
		{"ru", "ru"},
		{"ru-RU", "ru"},
		{"RU", "ru"},
		{"ru-RU,ru;q=0.9,en;q=0.8", "ru"},
		{"en-US,en;q=0.9", "en"},
		{"fr-FR", "en"},
		{"  ru-RU  ", "ru"},
		{"!!!", "en"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest("POST", "/", nil)
		if tc.header != "" {
			req.Header.Set("Accept-Language", tc.header)
		}
		got := LocaleFromRequest(req)
		if got != tc.want {
			t.Errorf("LocaleFromRequest(%q) = %q, want %q", tc.header, got, tc.want)
		}
	}
}
