package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// TestOnlyGitkeep_TrueWhenOnlyGitkeep verifies the helper detects a dist
// directory containing only the .gitkeep placeholder.
func TestOnlyGitkeep_TrueWhenOnlyGitkeep(t *testing.T) {
	syntheticFS := fstest.MapFS{
		".gitkeep": &fstest.MapFile{Data: []byte("placeholder\n")},
	}
	if !onlyGitkeep(syntheticFS) {
		t.Fatalf("expected onlyGitkeep to return true for FS containing only .gitkeep")
	}
}

// TestOnlyGitkeep_FalseWhenIndexHTMLPresent verifies the helper detects real
// build artifacts and returns false so the SPA path is exercised.
func TestOnlyGitkeep_FalseWhenIndexHTMLPresent(t *testing.T) {
	syntheticFS := fstest.MapFS{
		".gitkeep":   &fstest.MapFile{Data: []byte("placeholder\n")},
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><html></html>")},
	}
	if onlyGitkeep(syntheticFS) {
		t.Fatalf("expected onlyGitkeep to return false when index.html is present")
	}
}

// TestFallbackHandler_Returns503 ensures the fallback responds with a 503
// status code and a body that explains the missing build step.
func TestFallbackHandler_Returns503(t *testing.T) {
	h := fallbackHandler()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Frontend assets not built") {
		t.Fatalf("body did not mention build instructions: %q", body)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("expected text/plain Content-Type, got %q", ct)
	}
}

// TestNewSPAHandler_FallbackViaPublicPath exercises the fallback path through
// the helper used by NewSPAHandler when only .gitkeep is present in the FS.
func TestNewSPAHandler_FallbackViaPublicPath(t *testing.T) {
	sub := fstest.MapFS{".gitkeep": &fstest.MapFile{Data: []byte("placeholder")}}
	h := newSPAHandlerFromFS(sub, "")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Frontend assets not built") {
		t.Errorf("body missing fallback message: %q", rr.Body.String())
	}
}

// TestNewSPAHandler_HappyPathViaInjectedFS exercises the SPA path with an
// injected synthetic FS containing real-looking assets.
func TestNewSPAHandler_HappyPathViaInjectedFS(t *testing.T) {
	sub := fstest.MapFS{
		"index.html":     &fstest.MapFile{Data: []byte("<!doctype html><html>app</html>")},
		"assets/main.js": &fstest.MapFile{Data: []byte("console.log('hi')")},
	}
	h := newSPAHandlerFromFS(sub, "")

	// Root returns index.html with CSP headers.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "<html>app</html>") {
		t.Errorf("missing index.html body: %q", rr.Body.String())
	}
	if !strings.Contains(rr.Header().Get("Content-Security-Policy"), "default-src 'self'") {
		t.Errorf("CSP header missing or wrong: %q", rr.Header().Get("Content-Security-Policy"))
	}
	if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("X-Content-Type-Options missing")
	}
	if rr.Header().Get("X-Frame-Options") != "DENY" {
		t.Errorf("X-Frame-Options missing")
	}

	// Unknown path falls back to index.html (SPA routing).
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/some/route", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("SPA fallback expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "<html>app</html>") {
		t.Errorf("SPA fallback did not return index.html: %q", rr.Body.String())
	}
}

// TestNewSPAHandler_CSPIncludesBillingURL ensures connect-src contains the
// billing origin so the SPA can fetch cross-origin in cloud deployments.
func TestNewSPAHandler_CSPIncludesBillingURL(t *testing.T) {
	sub := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><html>app</html>")},
	}
	h := newSPAHandlerFromFS(sub, "https://billing.agentorbit.tech")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	csp := rr.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "connect-src 'self' ws: wss: https://billing.agentorbit.tech") {
		t.Errorf("expected billing origin in connect-src, got %q", csp)
	}
}

// TestNewSPAHandler_RealAssetsPath exercises the happy path: with real built
// assets present in the embedded dist directory, the handler should serve
// index.html on a GET / request and emit security headers.
func TestNewSPAHandler_RealAssetsPath(t *testing.T) {
	// Pre-flight: the embedded dist must contain real assets for this test.
	sub, err := fs.Sub(embeddedWebFS, "dist")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}
	if onlyGitkeep(sub) {
		t.Skip("dist/ contains only .gitkeep — skipping happy-path test (run 'make web' first)")
	}

	h, err := NewSPAHandler("")
	if err != nil {
		t.Fatalf("NewSPAHandler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /, got %d (body: %q)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(strings.ToLower(rr.Body.String()), "<!doctype html") &&
		!strings.Contains(strings.ToLower(rr.Body.String()), "<html") {
		t.Fatalf("expected HTML content in response body, got: %q", rr.Body.String())
	}
	if csp := rr.Header().Get("Content-Security-Policy"); csp == "" {
		t.Fatalf("expected Content-Security-Policy header to be set on SPA path")
	}
	if x := rr.Header().Get("X-Content-Type-Options"); x != "nosniff" {
		t.Fatalf("expected X-Content-Type-Options=nosniff, got %q", x)
	}
	if x := rr.Header().Get("X-Frame-Options"); x != "DENY" {
		t.Fatalf("expected X-Frame-Options=DENY, got %q", x)
	}
}
