package handler

import (
	"net/http"
	"strings"
)

// LocaleFromRequest returns the best-supported locale for the request based on
// the Accept-Language header. Returns "en" or "ru" only; defaults to "en".
//
// Parses the first language tag only (quality values are not evaluated — the
// first listed language wins). This matches the UI's behavior where the user
// actively chose a language in the frontend and we want to honor that choice
// without ambiguity.
func LocaleFromRequest(r *http.Request) string {
	raw := r.Header.Get("Accept-Language")
	if raw == "" {
		return "en"
	}
	// Take the first tag before any comma (skip q-values entirely).
	first := raw
	if i := strings.IndexByte(raw, ','); i >= 0 {
		first = raw[:i]
	}
	// Strip any q-value (";q=...").
	if i := strings.IndexByte(first, ';'); i >= 0 {
		first = first[:i]
	}
	// Primary subtag only ("ru-RU" -> "ru").
	if i := strings.IndexByte(first, '-'); i >= 0 {
		first = first[:i]
	}
	first = strings.ToLower(strings.TrimSpace(first))
	switch first {
	case "ru":
		return "ru"
	default:
		return "en"
	}
}
