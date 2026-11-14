//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"lokasolve/services/api-go/internal/platform/authn"
)

func TestOverlappingRefreshJoinsOneRotation(t *testing.T) {
	reset(t)
	first := register(t, "overlap@example.com", "resident", "", "")
	const n = 5
	start := make(chan struct{})
	var wg sync.WaitGroup
	codes := make([]int, n)
	bodies := make([]sessionBody, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			req, err := http.NewRequest(http.MethodPost, api.URL+"/api/v1/auth/refresh", strings.NewReader(`{"refresh_token":"`+first.RefreshToken+`"}`))
			if err != nil {
				errs[i] = err
				return
			}
			req.Header.Set("Content-Type", "application/json")
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				errs[i] = err
				return
			}
			defer res.Body.Close()
			codes[i] = res.StatusCode
			errs[i] = json.NewDecoder(res.Body).Decode(&bodies[i])
		}(i)
	}
	close(start)
	wg.Wait()
	for i := 0; i < n; i++ {
		if errs[i] != nil || codes[i] != http.StatusOK {
			t.Fatalf("refresh %d status %d err %v", i, codes[i], errs[i])
		}
		if bodies[i].RefreshToken != bodies[0].RefreshToken {
			t.Fatal("overlapping refreshes minted different sessions")
		}
		authed(t, http.MethodGet, "/api/v1/me", bodies[i].AccessToken, nil, http.StatusOK)
	}
	authed(t, http.MethodGet, "/api/v1/me", first.AccessToken, nil, http.StatusUnauthorized)
	var active, total int
	if err := adminDB.QueryRow(context.Background(), `
		SELECT count(*) FILTER (WHERE revoked_at IS NULL), count(*)
		FROM sessions WHERE user_id = (SELECT id FROM users WHERE email = 'overlap@example.com')
	`).Scan(&active, &total); err != nil {
		t.Fatal(err)
	}
	if active != 1 || total != 2 {
		t.Fatalf("active sessions %d total %d", active, total)
	}
}

func TestRefreshRejectsInvalidExpiredAndLoggedOutTokens(t *testing.T) {
	reset(t)
	first := register(t, "expire@example.com", "resident", "", "")
	other := login(t, "expire@example.com")
	if post(t, "/api/v1/auth/refresh", map[string]string{"refresh_token": "not-a-token"}, "") != http.StatusUnauthorized {
		t.Fatal("invalid token was accepted")
	}
	if _, err := adminDB.Exec(context.Background(), `
		UPDATE sessions SET expires_at = now() - interval '1 minute' WHERE refresh_token_hash = $1
	`, authn.HashRefresh(first.RefreshToken)); err != nil {
		t.Fatal(err)
	}
	if post(t, "/api/v1/auth/refresh", map[string]string{"refresh_token": first.RefreshToken}, "") != http.StatusUnauthorized {
		t.Fatal("expired token was accepted")
	}
	authed(t, http.MethodGet, "/api/v1/me", other.AccessToken, nil, http.StatusOK)
	rotated := refresh(t, other.RefreshToken)
	logout := post(t, "/api/v1/auth/logout", map[string]string{"refresh_token": rotated.RefreshToken}, "")
	if logout != http.StatusNoContent {
		t.Fatalf("logout %d", logout)
	}
	authed(t, http.MethodGet, "/api/v1/me", rotated.AccessToken, nil, http.StatusUnauthorized)
	if post(t, "/api/v1/auth/refresh", map[string]string{"refresh_token": rotated.RefreshToken}, "") != http.StatusUnauthorized {
		t.Fatal("logged-out refresh was accepted")
	}
}

func TestSeverityReviewAndHonestReferral(t *testing.T) {
	reset(t)
	owner := register(t, "owner@example.com", "resident", "", "")
	other := register(t, "other@example.com", "resident", "", "")
	support := insertStaff(t, "support@example.com", "support_agent")
	admin := insertStaff(t, "root@example.com", "super_admin")

	created := authed(t, http.MethodPost, "/api/v1/issues", owner.AccessToken, map[string]any{
		"description":    "Exposed live wire on the lane near the school",
		"locality_text":  "Ward 12",
		"stated_urgency": "normal",
	}, http.StatusCreated)
	issue, _ := created["issue"].(map[string]any)
	issueID, _ := issue["id"].(string)
	severity, _ := issue["severity"].(map[string]any)
	if severity["level"] != "P0" || severity["source"] != "rule" {
		t.Fatalf("severity = %#v", severity)
	}
	safety, _ := created["safety"].(map[string]any)
	message, _ := safety["message"].(string)
	if safety["dispatched"] != false || !strings.Contains(message, "has not dispatched") || !strings.Contains(message, "112") {
		t.Fatalf("safety = %#v", safety)
	}

	reviews := authed(t, http.MethodGet, "/api/v1/admin/reviews", support.AccessToken, nil, http.StatusOK)
	data, _ := reviews["data"].([]any)
	if len(data) == 0 {
		t.Fatal("serious report did not enter review")
	}
	firstReview, _ := data[0].(map[string]any)
	if !strings.Contains(fmtReason(firstReview), "No authorized submission") && !strings.Contains(fmtReason(firstReview), "assess") {
		t.Fatalf("review = %#v", firstReview)
	}
	authed(t, http.MethodPost, "/api/v1/admin/reviews/"+firstReview["id"].(string)+"/decision", owner.AccessToken, map[string]string{
		"decision": "confirm_routing", "note": "I am the resident",
	}, http.StatusForbidden)
	authed(t, http.MethodPost, "/api/v1/admin/reviews/"+firstReview["id"].(string)+"/decision", support.AccessToken, map[string]string{
		"decision": "reject_classification", "note": "A person checked the report. It is routine lighting.", "severity": "P3",
	}, http.StatusNoContent)
	events := authed(t, http.MethodGet, "/api/v1/admin/reviews/"+firstReview["id"].(string)+"/events", support.AccessToken, nil, http.StatusOK)
	if len(events["data"].([]any)) != 1 {
		t.Fatalf("events = %#v", events)
	}
	seen := authed(t, http.MethodGet, "/api/v1/issues/"+issueID, owner.AccessToken, nil, http.StatusOK)
	overridden, _ := seen["severity"].(map[string]any)
	if overridden["level"] != "P3" || overridden["overridden"] != true || overridden["source"] != "reviewer" {
		t.Fatalf("override = %#v", overridden)
	}

	plain := authed(t, http.MethodPost, "/api/v1/issues", owner.AccessToken, map[string]any{
		"description":   "The courtyard bench is loose and needs a look",
		"locality_text": "Ward 12",
	}, http.StatusCreated)
	plainIssue, _ := plain["issue"].(map[string]any)
	plainID, _ := plainIssue["id"].(string)
	queue := authed(t, http.MethodGet, "/api/v1/admin/reviews", support.AccessToken, nil, http.StatusOK)
	if !strings.Contains(mustJSONString(queue), "No responsible authority") {
		t.Fatal("unmatched report was not held for review")
	}
	authed(t, http.MethodGet, "/api/v1/issues/"+plainID+"/progress", other.AccessToken, nil, http.StatusNotFound)

	office := authed(t, http.MethodPost, "/api/v1/admin/authorities", admin.AccessToken, map[string]string{
		"official_name": "Ward Works", "org_type": "department", "locality_text": "Ward 12",
		"categories": "Lighting", "verification_source": "Office sign recorded by the operator",
	}, http.StatusCreated)
	parent := authed(t, http.MethodPost, "/api/v1/admin/authorities", admin.AccessToken, map[string]string{
		"official_name": "City Works", "org_type": "department", "locality_text": "City",
		"categories": "", "verification_source": "Operator directory note",
	}, http.StatusCreated)
	referral := authed(t, http.MethodPost, "/api/v1/issues/"+issueID+"/referrals", owner.AccessToken, map[string]any{
		"authority_id": office["id"], "consent": true,
	}, http.StatusCreated)
	referralID, _ := referral["id"].(string)
	progress := authed(t, http.MethodGet, "/api/v1/issues/"+issueID+"/progress", owner.AccessToken, nil, http.StatusOK)
	if progress["packet_prepared"] != true || progress["submitted"] != false || progress["delivered"] != false || progress["official_reference"] != nil {
		t.Fatalf("prepared progress = %#v", progress)
	}
	authed(t, http.MethodPost, "/api/v1/referrals/"+referralID+"/advance", admin.AccessToken, map[string]string{
		"status": "delivered", "note": "sent",
	}, http.StatusBadRequest)
	authed(t, http.MethodPost, "/api/v1/referrals/"+referralID+"/advance", admin.AccessToken, map[string]string{
		"status": "approved_for_submission",
	}, http.StatusNoContent)
	authed(t, http.MethodPost, "/api/v1/referrals/"+referralID+"/advance", admin.AccessToken, map[string]string{
		"status": "manual_handoff", "note": "Handed to the desk clerk with the printed packet.",
	}, http.StatusNoContent)
	authed(t, http.MethodPost, "/api/v1/referrals/"+referralID+"/advance", admin.AccessToken, map[string]string{
		"status": "acknowledged", "note": "The clerk said it was received.",
	}, http.StatusNoContent)
	authed(t, http.MethodPost, "/api/v1/referrals/"+referralID+"/advance", admin.AccessToken, map[string]string{
		"status": "officially_registered", "external_reference": "OP-1", "note": "Typed from a phone call",
	}, http.StatusNoContent)
	var prepared, submitted, delivered, officialAck bool
	var referenceKind *string
	if err := adminDB.QueryRow(context.Background(), `
		SELECT packet_prepared_at IS NOT NULL, submission_attempted_at IS NOT NULL,
			delivered_at IS NOT NULL, official_acknowledged_at IS NOT NULL, reference_kind
		FROM referrals WHERE id = $1
	`, referralID).Scan(&prepared, &submitted, &delivered, &officialAck, &referenceKind); err != nil {
		t.Fatal(err)
	}
	if !prepared || submitted || delivered || officialAck || referenceKind == nil || *referenceKind != "operator_entered" {
		t.Fatalf("prepared %v submitted %v delivered %v ack %v kind %v", prepared, submitted, delivered, officialAck, referenceKind)
	}
	after := authed(t, http.MethodGet, "/api/v1/issues/"+issueID+"/progress", owner.AccessToken, nil, http.StatusOK)
	if after["submitted"] != false || after["delivered"] != false || after["officially_acknowledged"] != false || after["operator_reference"] != "OP-1" || after["official_reference"] != nil {
		t.Fatalf("progress after reference = %#v", after)
	}
	if after["escalation_due_at"] == nil {
		t.Fatal("no escalation deadline")
	}
	authed(t, http.MethodPost, "/api/v1/referrals/"+referralID+"/advance", owner.AccessToken, map[string]string{
		"status": "department_assigned", "note": "Lighting",
	}, http.StatusForbidden)

	if _, err := adminDB.Exec(context.Background(), `
		INSERT INTO escalation_steps (id, authority_id, stage, condition, target_authority_id, requires_review)
		VALUES ($1, $2, 1, 'deadline', $3, true)
	`, uuid.NewString(), office["id"], parent["id"]); err != nil {
		t.Fatal(err)
	}
	authed(t, http.MethodPost, "/api/v1/referrals/"+referralID+"/escalate", admin.AccessToken, nil, http.StatusNoContent)
	var due bool
	if err := adminDB.QueryRow(context.Background(), `
		SELECT escalation_due_at IS NOT NULL AND submission_attempted_at IS NULL FROM referrals WHERE id = $1
	`, referralID).Scan(&due); err != nil || !due {
		t.Fatalf("escalation deadline missing err %v", err)
	}
}

func fmtReason(item map[string]any) string {
	reason, _ := item["reason"].(string)
	return reason
}

func mustJSONString(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}

func TestRefreshDoesNotRetryAfterFailure(t *testing.T) {
	reset(t)
	res, err := http.Post(api.URL+"/api/v1/auth/refresh", "application/json", strings.NewReader(`{"refresh_token":"missing"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		raw, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d body %s", res.StatusCode, raw)
	}
}
