//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"lokasolve/services/api-go/internal/authority"
	"lokasolve/services/api-go/internal/platform/authn"
	"lokasolve/services/api-go/internal/workflow"
)

func TestSuggestionCannotAuthorizeAuthorityRouting(t *testing.T) {
	ctx := context.Background()
	reporter := uuid.New()
	issueID := uuid.New()
	officeID := uuid.New()
	orgID := uuid.New()
	resolverID := uuid.New()
	email := "route-" + reporter.String() + "@example.com"
	if _, err := adminDB.Exec(ctx, `INSERT INTO users (id, email) VALUES ($1, $2)`, reporter, email); err != nil {
		t.Fatal(err)
	}
	if _, err := adminDB.Exec(ctx, `INSERT INTO organizations (id, name, org_type) VALUES ($1, 'Market ward', 'municipality')`, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err := adminDB.Exec(ctx, `INSERT INTO organization_jurisdictions (organization_id, locality_text) VALUES ($1, 'Market lane')`, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err := adminDB.Exec(ctx, `
		INSERT INTO issue_projection (
			id, reporter_user_id, description, category_label, locality_text, status, severity,
			latitude, longitude, classification_provenance, suggested_label, created_at
		) VALUES ($1, $2, 'Saved during a taxonomy outage with a known type.', 'Power', 'Market lane', 'UNDER_REVIEW', 'P3', 17.4, 78.4, 'RULE_SUGGESTED', 'Power', now())
	`, issueID, reporter); err != nil {
		t.Fatal(err)
	}
	if _, err := adminDB.Exec(ctx, `
		INSERT INTO authorities (id, official_name, org_type, locality_text, categories, verification_status)
		VALUES ($1, 'Ward lighting desk', 'municipality', 'Market lane', 'Power, Lighting', 'verified')
	`, officeID); err != nil {
		t.Fatal(err)
	}
	if _, err := adminDB.Exec(ctx, `
		INSERT INTO authority_channels (id, authority_id, channel, destination, purpose, verification_status)
		VALUES ($1, $2, 'email', 'desk@example.com', 'complaint', 'verified')
	`, uuid.New(), officeID); err != nil {
		t.Fatal(err)
	}
	if _, err := adminDB.Exec(ctx, `
		INSERT INTO resolvers (id, display_name, kind, locality_text, skills, verification_status)
		VALUES ($1, 'Lane electrician', 'provider', 'Market lane', 'Power, Lighting', 'reviewed')
	`, resolverID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = adminDB.Exec(ctx, `DELETE FROM message_attempts WHERE issue_id = $1`, issueID)
		_, _ = adminDB.Exec(ctx, `DELETE FROM notifications WHERE issue_id = $1`, issueID)
		_, _ = adminDB.Exec(ctx, `DELETE FROM issue_events WHERE issue_id = $1`, issueID)
		_, _ = adminDB.Exec(ctx, `DELETE FROM referrals WHERE issue_id = $1`, issueID)
		_, _ = adminDB.Exec(ctx, `DELETE FROM assignments WHERE issue_id = $1`, issueID)
		_, _ = adminDB.Exec(ctx, `DELETE FROM review_items WHERE issue_id = $1`, issueID)
		_, _ = adminDB.Exec(ctx, `DELETE FROM issue_projection WHERE id = $1`, issueID)
		_, _ = adminDB.Exec(ctx, `DELETE FROM authority_channels WHERE authority_id = $1`, officeID)
		_, _ = adminDB.Exec(ctx, `DELETE FROM authorities WHERE id = $1`, officeID)
		_, _ = adminDB.Exec(ctx, `DELETE FROM resolvers WHERE id = $1`, resolverID)
		_, _ = adminDB.Exec(ctx, `DELETE FROM organization_jurisdictions WHERE organization_id = $1`, orgID)
		_, _ = adminDB.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, orgID)
		_, _ = adminDB.Exec(ctx, `DELETE FROM users WHERE id = $1`, reporter)
	})

	operator := authn.Principal{UserID: reporter, Permissions: map[string]struct{}{"admin:roles_grant": {}}}
	offices, err := (&authority.Service{DB: appDB}).Match(ctx, operator, issueID)
	if err != nil {
		t.Fatal(err)
	}
	if len(offices) != 0 {
		t.Fatalf("suggestion matched offices: %+v", offices)
	}
	if _, err := (&authority.Service{DB: appDB}).Prepare(ctx, operator, issueID, officeID, true); err == nil {
		t.Fatal("suggestion prepared a complaint")
	}
	if _, err := (&workflow.Service{DB: appDB}).Offer(ctx, operator, issueID, resolverID, "test"); err == nil {
		t.Fatal("suggestion offered a provider")
	}
	assertNoExternalSubmission(t, issueID)
	var reviews int
	if err := adminDB.QueryRow(ctx, `SELECT count(*) FROM review_items WHERE issue_id = $1 AND status = 'open'`, issueID).Scan(&reviews); err != nil {
		t.Fatal(err)
	}
	if reviews == 0 {
		t.Fatal("unverified classification did not stay in review")
	}

	if _, err := adminDB.Exec(ctx, `
		UPDATE issue_projection
		SET classification_provenance = 'USER_SELECTED_VALIDATED', category_label = 'Lighting', suggested_label = NULL
		WHERE id = $1
	`, issueID); err != nil {
		t.Fatal(err)
	}
	if _, err := adminDB.Exec(ctx, `UPDATE authorities SET locality_text = 'Other town' WHERE id = $1`, officeID); err != nil {
		t.Fatal(err)
	}
	offices, err = (&authority.Service{DB: appDB}).Match(ctx, operator, issueID)
	if err != nil {
		t.Fatal(err)
	}
	if len(offices) == 0 {
		t.Fatal("validated category produced no recommendation")
	}
	foundOffice := false
	for _, office := range offices {
		if office.RoutingAuthorized {
			t.Fatalf("jurisdiction mismatch was authorized: %+v", office)
		}
		if office.ID == officeID.String() {
			foundOffice = true
		}
	}
	if !foundOffice {
		t.Fatal("the verified office was not recommended")
	}
	if _, err := (&authority.Service{DB: appDB}).Prepare(ctx, operator, issueID, officeID, true); err == nil {
		t.Fatal("wrong jurisdiction prepared a complaint")
	}

	if _, err := adminDB.Exec(ctx, `UPDATE authorities SET locality_text = 'Market lane' WHERE id = $1`, officeID); err != nil {
		t.Fatal(err)
	}
	if _, err := adminDB.Exec(ctx, `UPDATE issue_projection SET severity = 'P0' WHERE id = $1`, issueID); err != nil {
		t.Fatal(err)
	}
	if _, err := (&authority.Service{DB: appDB}).Prepare(ctx, operator, issueID, officeID, true); err == nil {
		t.Fatal("unapproved severe report was routed")
	}
	if _, err := adminDB.Exec(ctx, `
		UPDATE review_items SET status = 'resolved', decision = 'confirm_routing', decision_note = 'A person checked the office.'
		WHERE issue_id = $1 AND status = 'open'
	`, issueID); err != nil {
		t.Fatal(err)
	}
	prepared, err := (&authority.Service{DB: appDB}).Prepare(ctx, operator, issueID, officeID, true)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Status != "consented" {
		t.Fatalf("status %s", prepared.Status)
	}
	assertNoExternalSubmission(t, issueID)
}

func assertNoExternalSubmission(t *testing.T, issueID uuid.UUID) {
	t.Helper()
	var sent, external int
	if err := adminDB.QueryRow(context.Background(), `
		SELECT count(*) FROM referrals WHERE issue_id = $1 AND status IN ('sent', 'delivered', 'officially_registered')
	`, issueID).Scan(&sent); err != nil {
		t.Fatal(err)
	}
	if err := adminDB.QueryRow(context.Background(), `
		SELECT count(*) FROM message_attempts
		WHERE issue_id = $1 AND status NOT IN ('recorded', 'not_configured')
	`, issueID).Scan(&external); err != nil {
		t.Fatal(err)
	}
	if sent != 0 || external != 0 {
		t.Fatalf("external submission sent %d messages %d", sent, external)
	}
}
