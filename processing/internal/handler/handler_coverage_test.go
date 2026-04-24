//go:build integration

package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- Auth handler coverage ---

func TestHandler_VerifyEmail_InvalidToken(t *testing.T) {
	env := setupTestEnv(t)
	body := jsonBody(t, map[string]string{"token": "totally-bogus-not-a-token"})
	req := httptest.NewRequest("POST", "/auth/verify-email", body)
	req.Header.Set("Content-Type", "application/json")
	rr := do(t, env, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandler_VerifyEmail_InvalidJSON_V2(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest("POST", "/auth/verify-email", bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")
	rr := do(t, env, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandler_ResetPassword_InvalidToken(t *testing.T) {
	env := setupTestEnv(t)
	body := jsonBody(t, map[string]string{"token": "bogus", "password": "NewPassword1"})
	req := httptest.NewRequest("POST", "/auth/reset-password", body)
	req.Header.Set("Content-Type", "application/json")
	rr := do(t, env, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandler_ResetPassword_InvalidJSON_V2(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest("POST", "/auth/reset-password", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rr := do(t, env, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandler_Login_InvalidJSON_V2(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest("POST", "/auth/login", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rr := do(t, env, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandler_RequestPasswordReset_InvalidJSON_V2(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest("POST", "/auth/request-password-reset", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rr := do(t, env, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// --- Org handler coverage: list with empty, invalid settings body ---

// ListOrgs should return the organization auto-provisioned at signup. Previously
// this test expected an empty list, which was only possible because registration
// did not create the org — fixed along with the dashboard "create org" re-prompt.
func TestHandler_ListOrgs_ReturnsSignupOrg(t *testing.T) {
	env := setupTestEnv(t)
	token := registerAndLogin(t, env, "emptylist@example.com", "Empty", "Password1")

	rr := do(t, env, authReq(t, "GET", "/orgs", nil, token))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var orgs []interface{}
	if err := json.NewDecoder(rr.Body).Decode(&orgs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(orgs) != 1 {
		t.Errorf("expected 1 org provisioned by signup, got %d", len(orgs))
	}
}

func TestHandler_UpdateSettings_InvalidJSON_V2(t *testing.T) {
	env := setupTestEnv(t)
	token := registerAndLogin(t, env, "settings-bad@example.com", "SB", "Password1")
	orgID := createOrg(t, env, token, "SB Org")

	req := httptest.NewRequest("PUT", "/orgs/"+orgID+"/settings", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	rr := do(t, env, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --- Invite handler ---

func TestHandler_Invite_ListEmpty(t *testing.T) {
	env := setupTestEnv(t)
	token := registerAndLogin(t, env, "inv-empty@example.com", "IE", "Password1")
	orgID := createOrg(t, env, token, "IE Org")

	rr := do(t, env, authReq(t, "GET", "/orgs/"+orgID+"/invites", nil, token))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandler_AcceptInvite_InvalidJSON_V2(t *testing.T) {
	env := setupTestEnv(t)
	token := registerAndLogin(t, env, "accept-bad@example.com", "AB", "Password1")

	req := httptest.NewRequest("POST", "/accept-invite", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	rr := do(t, env, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandler_AcceptInvite_BadToken(t *testing.T) {
	env := setupTestEnv(t)
	token := registerAndLogin(t, env, "accept-bt@example.com", "AB", "Password1")

	rr := do(t, env, authReq(t, "POST", "/accept-invite", map[string]string{"token": "bogus-invite-token"}, token))
	if rr.Code < 400 {
		t.Errorf("expected 4xx, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --- Unauthorized access ---

func TestHandler_Unauthenticated_GetMe(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest("GET", "/user/me", nil)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	rr := do(t, env, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestHandler_Unauthenticated_ListOrgs(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest("GET", "/orgs", nil)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	rr := do(t, env, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestHandler_BadToken_GetMe(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest("GET", "/user/me", nil)
	req.Header.Set("Authorization", "Bearer not-a-valid-jwt")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	rr := do(t, env, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// --- Internal endpoints coverage: wrong token, malformed ingest ---

func TestHandler_Internal_VerifyEmptyDigest(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest("POST", "/internal/auth/verify", jsonBody(t, map[string]string{"api_key_digest": ""}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", testInternalToken)
	rr := do(t, env, req)
	// Empty digest → valid=false (not found)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var body map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&body)
	if body["valid"] != false {
		t.Errorf("expected valid=false, got %+v", body)
	}
}

func TestHandler_Internal_VerifyInvalidJSON(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest("POST", "/internal/auth/verify", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", testInternalToken)
	rr := do(t, env, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandler_Internal_IngestInvalidJSON(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest("POST", "/internal/spans/ingest", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", testInternalToken)
	rr := do(t, env, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandler_Internal_MissingToken(t *testing.T) {
	env := setupTestEnv(t)
	req := httptest.NewRequest("POST", "/internal/auth/verify", jsonBody(t, map[string]string{"api_key_digest": "x"}))
	req.Header.Set("Content-Type", "application/json")
	// no X-Internal-Token
	rr := do(t, env, req)
	if rr.Code != http.StatusUnauthorized && rr.Code != http.StatusForbidden {
		t.Errorf("expected 401/403, got %d", rr.Code)
	}
}

// --- ChangePassword missing body / validation ---

func TestHandler_ChangePassword_InvalidJSON_V2(t *testing.T) {
	env := setupTestEnv(t)
	token := registerAndLogin(t, env, "ch-pw-bad@example.com", "User", "Password1")

	req := httptest.NewRequest("PUT", "/user/password", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	rr := do(t, env, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandler_UpdateProfile_InvalidJSON_V2(t *testing.T) {
	env := setupTestEnv(t)
	token := registerAndLogin(t, env, "up-bad@example.com", "U", "Password1")

	req := httptest.NewRequest("PUT", "/user/profile", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	rr := do(t, env, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// --- TransferOwnership invalid cases ---

func TestHandler_TransferOwnership_InvalidUserID(t *testing.T) {
	env := setupTestEnv(t)
	token := registerAndLogin(t, env, "xfer-bad@example.com", "XB", "Password1")
	orgID := createOrg(t, env, token, "XB Org")

	rr := do(t, env, authReq(t, "POST", "/orgs/"+orgID+"/transfer", map[string]string{"new_owner_user_id": "not-a-uuid"}, token))
	if rr.Code != http.StatusBadRequest && rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 400/422, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandler_TransferOwnership_InvalidJSON(t *testing.T) {
	env := setupTestEnv(t)
	token := registerAndLogin(t, env, "xfer-json@example.com", "XJ", "Password1")
	orgID := createOrg(t, env, token, "XJ Org")

	req := httptest.NewRequest("POST", "/orgs/"+orgID+"/transfer", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	rr := do(t, env, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// --- InitiateDeletion / CancelDeletion flow ---

func TestHandler_InitiateAndCancelDeletion(t *testing.T) {
	env := setupTestEnv(t)
	token := registerAndLogin(t, env, "del-flow@example.com", "DF", "Password1")
	orgID := createOrg(t, env, token, "DF Org")

	// Initiate deletion
	rr := do(t, env, authReq(t, "DELETE", "/orgs/"+orgID, nil, token))
	if rr.Code != http.StatusOK && rr.Code != http.StatusAccepted && rr.Code != http.StatusNoContent {
		t.Fatalf("expected 2xx on initiate deletion, got %d: %s", rr.Code, rr.Body.String())
	}

	// Cancel deletion
	rr = do(t, env, authReq(t, "POST", "/orgs/"+orgID+"/restore", nil, token))
	if rr.Code != http.StatusOK && rr.Code != http.StatusNoContent {
		t.Fatalf("expected 2xx on restore, got %d: %s", rr.Code, rr.Body.String())
	}
}
