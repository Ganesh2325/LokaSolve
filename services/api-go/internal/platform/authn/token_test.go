package authn

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAccessTokenHasNoRoleClaims(t *testing.T) {
	mgr := NewTokenManager("test-token-secret-must-be-at-least-32", time.Minute, time.Hour)
	userID := uuid.New()
	sessionID := uuid.New()
	raw, err := mgr.IssueAccess(userID, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		t.Fatalf("token parts: %d", len(parts))
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(body, &claims); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"role", "roles", "permissions", "tenant_id", "email"} {
		if _, ok := claims[forbidden]; ok {
			t.Fatalf("token contains %s", forbidden)
		}
	}
	gotUser, gotSession, err := mgr.ParseAccess(raw)
	if err != nil {
		t.Fatal(err)
	}
	if gotUser != userID || gotSession != sessionID {
		t.Fatalf("parsed ids mismatch")
	}
	if _, _, err := mgr.ParseAccess(raw + "tamper"); err == nil {
		t.Fatal("tampered token was accepted")
	}
}
