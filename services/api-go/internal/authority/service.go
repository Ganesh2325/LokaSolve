package authority

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"lokasolve/services/api-go/internal/identity"
	"lokasolve/services/api-go/internal/issueclient"
	"lokasolve/services/api-go/internal/platform/audit"
	"lokasolve/services/api-go/internal/platform/authn"
	"lokasolve/services/api-go/internal/platform/database"
	"lokasolve/services/api-go/internal/platform/httpx"
	"lokasolve/services/api-go/internal/workflow"
)

type Service struct {
	DB    *database.DB
	Issue *issueclient.Client
}

type Office struct {
	ID                 string  `json:"id"`
	OfficialName       string  `json:"official_name"`
	OrgType            string  `json:"org_type"`
	LocalityText       *string `json:"locality_text"`
	Categories         string  `json:"categories"`
	VerificationStatus string  `json:"verification_status"`
	VerificationSource *string `json:"verification_source"`
	Explanation        string  `json:"explanation,omitempty"`
	RoutingAuthorized  bool    `json:"routing_authorized"`
}

type Channel struct {
	ID                 string  `json:"id"`
	Channel            string  `json:"channel"`
	Destination        *string `json:"destination,omitempty"`
	Purpose            *string `json:"purpose"`
	VerificationStatus string  `json:"verification_status"`
}

type Referral struct {
	ID                string     `json:"id"`
	IssueID           string     `json:"issue_id"`
	AuthorityID       string     `json:"authority_id"`
	AuthorityName     string     `json:"authority_name"`
	Status            string     `json:"status"`
	Channel           string     `json:"channel"`
	ExternalReference *string    `json:"external_reference"`
	ConsentAt         *time.Time `json:"consent_at"`
	CreatedAt         time.Time  `json:"created_at"`
}

func operator(principal authn.Principal) bool {
	return principal.Has("admin:roles_grant") || principal.Has("admin:taxonomy_manage")
}

func (s *Service) AddOffice(ctx context.Context, principal authn.Principal, name, orgType, locality, categories, source, requestID string) (Office, error) {
	if !operator(principal) {
		return Office{}, httpx.New(403, "forbidden", "You do not have access to that action.")
	}
	name = strings.TrimSpace(name)
	orgType = strings.TrimSpace(orgType)
	locality = strings.TrimSpace(locality)
	categories = strings.TrimSpace(categories)
	source = strings.TrimSpace(source)
	if orgType == "" {
		orgType = "department"
	}
	if len([]rune(name)) < 2 {
		return Office{}, httpx.Validation(map[string]string{"official_name": "Use the official organization name."})
	}
	if source == "" {
		return Office{}, httpx.Validation(map[string]string{"verification_source": "Record where this office record came from."})
	}
	var localityValue, sourceValue *string
	localityValue = nil
	if locality != "" {
		localityValue = &locality
	}
	sourceValue = &source
	office := Office{
		ID: uuid.NewString(), OfficialName: name, OrgType: orgType, LocalityText: localityValue,
		Categories: categories, VerificationStatus: "pending", VerificationSource: sourceValue,
	}
	err := s.DB.WithFlags(ctx, &principal.UserID, []string{"app.platform_admin"}, func(tx pgx.Tx) error {
		id := uuid.MustParse(office.ID)
		if _, err := tx.Exec(ctx, `
			INSERT INTO authorities (id, official_name, org_type, locality_text, categories, verification_source)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, id, name, orgType, localityValue, categories, source); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{
			ActorID: &principal.UserID, ActorRole: "operator", Action: "authority.create",
			ResourceType: "authority", ResourceID: &id, RequestID: requestID,
			Metadata: map[string]any{"name": name, "verification_status": "pending"},
		})
	})
	return office, err
}

func (s *Service) AddChannel(ctx context.Context, principal authn.Principal, authorityID uuid.UUID, channel, destination, purpose string) (Channel, error) {
	if !operator(principal) {
		return Channel{}, httpx.New(403, "forbidden", "You do not have access to that action.")
	}
	channel = strings.TrimSpace(channel)
	switch channel {
	case "email", "whatsapp", "sms", "portal", "manual":
	default:
		return Channel{}, httpx.Validation(map[string]string{"channel": "Use email, whatsapp, sms, portal, or manual."})
	}
	destination = strings.TrimSpace(destination)
	purpose = strings.TrimSpace(purpose)
	var destinationValue, purposeValue *string
	if destination != "" {
		destinationValue = &destination
	}
	if purpose != "" {
		purposeValue = &purpose
	}
	item := Channel{ID: uuid.NewString(), Channel: channel, Destination: destinationValue, Purpose: purposeValue, VerificationStatus: "pending"}
	err := s.DB.WithFlags(ctx, &principal.UserID, []string{"app.platform_admin"}, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO authority_channels (id, authority_id, channel, destination, purpose)
			VALUES ($1, $2, $3, $4, $5)
		`, uuid.MustParse(item.ID), authorityID, channel, destinationValue, purposeValue)
		return err
	})
	return item, err
}

func (s *Service) Verify(ctx context.Context, principal authn.Principal, authorityID uuid.UUID, status string) error {
	if !operator(principal) {
		return httpx.New(403, "forbidden", "You do not have access to that action.")
	}
	if status != "verified" && status != "pending" && status != "unavailable" {
		return httpx.Validation(map[string]string{"verification_status": "Use pending, verified, or unavailable."})
	}
	return s.DB.WithFlags(ctx, &principal.UserID, []string{"app.platform_admin"}, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE authorities SET verification_status = $2 WHERE id = $1`, authorityID, status)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.New(404, "not_found", "That office is not available.")
		}
		return nil
	})
}

func (s *Service) ListOffices(ctx context.Context, principal authn.Principal) ([]Office, error) {
	if !principal.Has("admin:users_read") && !principal.Has("admin:roles_grant") && !principal.Has("admin:audit_read") {
		return nil, httpx.New(403, "forbidden", "You do not have access to that action.")
	}
	return s.offices(ctx, principal, "")
}

func (s *Service) offices(ctx context.Context, principal authn.Principal, where string, args ...any) ([]Office, error) {
	items := []Office{}
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		query := `SELECT id::text, official_name, org_type, locality_text, categories, verification_status, verification_source FROM authorities`
		if where != "" {
			query += " WHERE " + where
		}
		query += " ORDER BY official_name"
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item Office
			if err := rows.Scan(&item.ID, &item.OfficialName, &item.OrgType, &item.LocalityText, &item.Categories, &item.VerificationStatus, &item.VerificationSource); err != nil {
				return err
			}
			items = append(items, item)
		}
		return rows.Err()
	})
	return items, err
}

func (s *Service) Match(ctx context.Context, principal authn.Principal, issueID uuid.UUID) ([]Office, error) {
	found := []Office{}
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		report, err := workflow.LoadRouteReport(ctx, tx, issueID)
		if err == pgx.ErrNoRows {
			return httpx.New(404, "not_found", "That report is not available.")
		}
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id::text, official_name, org_type, locality_text, categories, verification_status, verification_source,
				EXISTS (
					SELECT 1 FROM authority_channels c
					WHERE c.authority_id = authorities.id
						AND c.verification_status = 'verified'
						AND c.destination IS NOT NULL
						AND btrim(c.destination) <> ''
				)
			FROM authorities ORDER BY official_name
		`)
		if err != nil {
			return err
		}
		defer rows.Close()
		allowed := false
		for rows.Next() {
			var item Office
			var channelReady bool
			if err := rows.Scan(&item.ID, &item.OfficialName, &item.OrgType, &item.LocalityText, &item.Categories, &item.VerificationStatus, &item.VerificationSource, &channelReady); err != nil {
				return err
			}
			decision := workflow.DecideRoute(report, officeTarget(item, report, channelReady))
			if !decision.Recommend {
				continue
			}
			item.Explanation = strings.Join(decision.Reasons, " ")
			item.RoutingAuthorized = decision.Allow
			item.VerificationSource = nil
			found = append(found, item)
			allowed = allowed || decision.Allow
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if !allowed {
			return workflow.OpenReview(ctx, tx, issueID, reviewReason(report))
		}
		return nil
	})
	return found, err
}

func officeTarget(item Office, report workflow.RouteReport, channelReady bool) workflow.RouteTarget {
	locality := ""
	if item.LocalityText != nil {
		locality = *item.LocalityText
	}
	return workflow.RouteTarget{
		CategoryMatch: workflow.CategoryListed(item.Categories, report.Category),
		LocalityMatch: workflow.SamePlace(report.Locality, locality),
		Verified:      item.VerificationStatus == "verified",
		ChannelReady:  channelReady,
	}
}

func reviewReason(report workflow.RouteReport) string {
	if report.Suggestion != "" {
		return "Suggested department: " + report.Suggestion + ". This is a suggestion for a person to review. No complaint was submitted and no provider was dispatched."
	}
	return "Routing conditions are not met. The report stays in human review. Nothing was sent."
}

func (s *Service) Prepare(ctx context.Context, principal authn.Principal, issueID, authorityID uuid.UUID, consent bool) (Referral, error) {
	if !consent {
		return Referral{}, httpx.New(400, "validation_error", "Consent is required before a complaint packet is prepared.")
	}
	var created Referral
	var held string
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		var description string
		var category, locality, address *string
		var share bool
		var reporter uuid.UUID
		err := tx.QueryRow(ctx, `
			SELECT description, category_label, locality_text, address_text, identity_visible_to_resolver, reporter_user_id
			FROM issue_visible($1)
		`, issueID).Scan(&description, &category, &locality, &address, &share, &reporter)
		if err == pgx.ErrNoRows {
			return httpx.New(404, "not_found", "That report is not available.")
		}
		if err != nil {
			return err
		}
		if reporter != principal.UserID && !operator(principal) {
			return httpx.New(403, "forbidden", "You do not have access to that action.")
		}
		var name, verification, categories string
		var officeLocality *string
		err = tx.QueryRow(ctx, `SELECT official_name, verification_status, categories, locality_text FROM authorities WHERE id = $1`, authorityID).Scan(&name, &verification, &categories, &officeLocality)
		if err == pgx.ErrNoRows {
			return httpx.New(404, "not_found", "That office is not available.")
		}
		if err != nil {
			return err
		}
		channel := "manual"
		_ = tx.QueryRow(ctx, `SELECT channel FROM authority_channels WHERE authority_id = $1 ORDER BY created_at LIMIT 1`, authorityID).Scan(&channel)
		report, err := workflow.LoadRouteReport(ctx, tx, issueID)
		if err != nil {
			return err
		}
		var channelReady bool
		_ = tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM authority_channels
				WHERE authority_id = $1 AND verification_status = 'verified'
					AND destination IS NOT NULL AND btrim(destination) <> ''
			)
		`, authorityID).Scan(&channelReady)
		decision := workflow.DecideRoute(report, officeTarget(Office{
			ID: authorityID.String(), Categories: categories, VerificationStatus: verification, LocalityText: officeLocality,
		}, report, channelReady))
		if !decision.Allow {
			held = reviewReason(report)
			return httpx.New(400, "validation_error", "Routing is not authorized. The report stays in human review. Nothing was sent.")
		}
		packet := map[string]any{
			"issue_id": issueID.String(), "summary": description, "category": category, "locality": locality,
			"address": address, "requested_action": "Review the reported problem.", "identity_shared": share,
			"authority_verification": verification, "user_claims_only": true,
		}
		raw, err := json.Marshal(packet)
		if err != nil {
			return err
		}
		id := uuid.New()
		now := time.Now().UTC()
		if _, err := tx.Exec(ctx, `
			INSERT INTO referrals (
				id, issue_id, authority_id, status, channel, consent_at, packet, packet_prepared_at, next_actor
			) VALUES ($1, $2, $3, 'consented', $4, $5, $6::jsonb, $5, 'operator')
		`, id, issueID, authorityID, channel, now, raw); err != nil {
			return err
		}
		if err := addEvent(ctx, tx, id, principal.UserID, "consented", "A complaint packet was prepared with consent. It has not been sent."); err != nil {
			return err
		}
		created = Referral{ID: id.String(), IssueID: issueID.String(), AuthorityID: authorityID.String(), AuthorityName: name, Status: "consented", Channel: channel, ConsentAt: &now, CreatedAt: now}
		return workflow.Record(ctx, tx, principal.UserID, issueID, reporter, "referral", "A complaint packet for "+name+" was prepared. Verification is "+verification+". Nothing was sent.", "Packet prepared")
	})
	if held != "" {
		_ = s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
			return workflow.OpenReview(ctx, tx, issueID, held)
		})
	}
	return created, err
}

func addEvent(ctx context.Context, tx pgx.Tx, referralID, actor uuid.UUID, status, body string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO referral_events (id, referral_id, actor_user_id, status, body) VALUES ($1, $2, $3, $4, $5)
	`, uuid.New(), referralID, actor, status, body)
	return err
}

func (s *Service) ListForIssue(ctx context.Context, principal authn.Principal, issueID uuid.UUID) ([]Referral, error) {
	return s.list(ctx, principal, "r.issue_id = $1", issueID)
}

func (s *Service) ListAll(ctx context.Context, principal authn.Principal) ([]Referral, error) {
	if !principal.Has("admin:users_read") && !principal.Has("admin:roles_grant") && !principal.Has("admin:audit_read") {
		return nil, httpx.New(403, "forbidden", "You do not have access to that action.")
	}
	return s.list(ctx, principal, "true")
}

func (s *Service) list(ctx context.Context, principal authn.Principal, where string, args ...any) ([]Referral, error) {
	items := []Referral{}
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT r.id::text, r.issue_id::text, r.authority_id::text, a.official_name, r.status, r.channel, r.external_reference, r.consent_at, r.created_at
			FROM referrals r JOIN authorities a ON a.id = r.authority_id
			WHERE `+where+` ORDER BY r.created_at DESC LIMIT 100
		`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item Referral
			if err := rows.Scan(&item.ID, &item.IssueID, &item.AuthorityID, &item.AuthorityName, &item.Status, &item.Channel, &item.ExternalReference, &item.ConsentAt, &item.CreatedAt); err != nil {
				return err
			}
			items = append(items, item)
		}
		return rows.Err()
	})
	return items, err
}

func (s *Service) Advance(ctx context.Context, principal authn.Principal, referralID uuid.UUID, next, note, reference string) error {
	next = strings.TrimSpace(next)
	note = strings.TrimSpace(note)
	reference = strings.TrimSpace(reference)
	blocked := map[string]bool{"sent": true, "provider_accepted": true, "delivered": true}
	if blocked[next] {
		return httpx.New(400, "validation_error", "That state needs evidence from the channel provider. It cannot be entered by hand.")
	}
	return s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		var current, channel string
		var issueID, reporter uuid.UUID
		err := tx.QueryRow(ctx, `
			SELECT r.status, r.channel, r.issue_id, i.reporter_user_id
			FROM referrals r JOIN LATERAL issue_visible(r.issue_id) i ON true WHERE r.id = $1
		`, referralID).Scan(&current, &channel, &issueID, &reporter)
		if err == pgx.ErrNoRows {
			return httpx.New(404, "not_found", "That referral is not available.")
		}
		if err != nil {
			return err
		}
		if !canAdvance(principal, reporter, current, next) {
			return httpx.New(403, "forbidden", "You do not have access to that action.")
		}
		if !allowed(current, next) {
			return httpx.New(400, "validation_error", "That status change is not available.")
		}
		if next == "officially_registered" && reference == "" {
			return httpx.Validation(map[string]string{"external_reference": "Enter the reference number you received. Do not invent one."})
		}
		if next == "manual_handoff" && note == "" {
			return httpx.Validation(map[string]string{"note": "Record who is carrying the packet and what evidence you have. This does not mark it delivered."})
		}
		if next == "department_assigned" && note == "" {
			return httpx.Validation(map[string]string{"note": "Record the department name you were given. Do not invent one."})
		}
		if (next == "acknowledged" || next == "rejected" || next == "failed" || next == "disputed") && note == "" {
			return httpx.Validation(map[string]string{"note": "Write what happened."})
		}
		body := honestAdvanceBody(next, channel, note, reference)
		var ref *string
		if reference != "" {
			ref = &reference
		}
		if _, err := tx.Exec(ctx, `
			UPDATE referrals SET
				status = $2,
				external_reference = COALESCE($3, external_reference),
				reference_kind = CASE WHEN $3::text IS NOT NULL THEN 'operator_entered' ELSE reference_kind END,
				authorized_at = CASE WHEN $2 = 'approved_for_submission' THEN now() ELSE authorized_at END,
				authorized_by = CASE WHEN $2 = 'approved_for_submission' THEN $4 ELSE authorized_by END,
				handoff_at = CASE WHEN $2 = 'manual_handoff' THEN now() ELSE handoff_at END,
				handoff_by = CASE WHEN $2 = 'manual_handoff' THEN $4 ELSE handoff_by END,
				handoff_channel = CASE WHEN $2 = 'manual_handoff' THEN channel ELSE handoff_channel END,
				handoff_evidence = CASE WHEN $2 = 'manual_handoff' THEN $5 ELSE handoff_evidence END,
				acknowledgement_kind = CASE WHEN $2 = 'acknowledged' THEN 'operator_note' ELSE acknowledgement_kind END,
				acknowledgement_note = CASE WHEN $2 IN ('acknowledged', 'rejected', 'failed') THEN $5 ELSE acknowledgement_note END,
				assigned_department = CASE WHEN $2 = 'department_assigned' THEN $5 ELSE assigned_department END,
				assignment_kind = CASE WHEN $2 = 'department_assigned' THEN 'operator_entered' ELSE assignment_kind END,
				next_actor = $6,
				updated_at = now()
			WHERE id = $1
		`, referralID, next, ref, principal.UserID, note, nextActor(next)); err != nil {
			return err
		}
		if next == "disputed" {
			if err := workflow.OpenReview(ctx, tx, issueID, "The referral was disputed. "+note); err != nil {
				return err
			}
		}
		if err := addEvent(ctx, tx, referralID, principal.UserID, next, body); err != nil {
			return err
		}
		return workflow.Record(ctx, tx, principal.UserID, issueID, reporter, "referral", body, "Referral updated")
	})
}

func nextActor(status string) string {
	switch status {
	case "resolution_reported":
		return "resident"
	case "not_supported", "rejected", "failed", "disputed":
		return "reviewer"
	case "confirmed", "closed":
		return "none"
	default:
		return "operator"
	}
}

func honestAdvanceBody(next, channel, note, reference string) string {
	switch next {
	case "approved_for_submission":
		return "An operator approved the packet for a future submission. Nothing was submitted."
	case "manual_handoff", "not_supported":
		return "No email, WhatsApp, or SMS provider is connected. A manual handoff was recorded on the " + channel + " channel. Nothing was delivered. " + note
	case "acknowledged":
		return "An operator wrote a note. This is not an official acknowledgement or a delivery receipt. " + note
	case "officially_registered":
		return "An operator entered reference " + reference + ". This is not a portal receipt, and nothing was marked delivered."
	case "department_assigned":
		return "An operator recorded a department name. The office has not confirmed the assignment. " + note
	case "action_started":
		return "An operator recorded that field action was described. LokaSolve has no field report from the office."
	case "resolution_reported":
		return "Resolution evidence was recorded. The resident has not verified it."
	case "confirmed":
		return "The resident confirmed the resolution."
	case "disputed":
		return "The resident disputed the resolution. " + note
	default:
		body := "Referral status is " + strings.ReplaceAll(next, "_", " ") + "."
		if note != "" {
			body += " " + note
		}
		return body
	}
}

func canAdvance(principal authn.Principal, reporter uuid.UUID, current, next string) bool {
	if operator(principal) {
		return true
	}
	return principal.UserID == reporter && (next == "confirmed" || next == "disputed")
}

func allowed(current, next string) bool {
	steps := map[string]map[string]bool{
		"consented":               {"approved_for_submission": true},
		"approved_for_submission": {"manual_handoff": true, "not_supported": true},
		"manual_handoff":          {"acknowledged": true, "rejected": true, "failed": true},
		"not_supported":           {"acknowledged": true, "rejected": true, "failed": true},
		"acknowledged":            {"officially_registered": true, "rejected": true},
		"officially_registered":   {"department_assigned": true},
		"department_assigned":     {"action_started": true},
		"action_started":          {"resolution_reported": true},
		"resolution_reported":     {"confirmed": true, "disputed": true},
		"disputed":                {"action_started": true},
		"confirmed":               {"closed": true},
	}
	return steps[current][next]
}

func (s *Service) Escalate(ctx context.Context, principal authn.Principal, referralID uuid.UUID) error {
	if !operator(principal) {
		return httpx.New(403, "forbidden", "You do not have access to that action.")
	}
	var heldIssue uuid.UUID
	var held string
	err := s.DB.WithFlags(ctx, &principal.UserID, []string{"app.platform_admin"}, func(tx pgx.Tx) error {
		var authorityID, issueID, reporter uuid.UUID
		var name string
		err := tx.QueryRow(ctx, `
			SELECT r.authority_id, r.issue_id, i.reporter_user_id, a.official_name
			FROM referrals r
			JOIN LATERAL issue_visible(r.issue_id) i ON true
			JOIN authorities a ON a.id = r.authority_id
			WHERE r.id = $1
		`, referralID).Scan(&authorityID, &issueID, &reporter, &name)
		if err == pgx.ErrNoRows {
			return httpx.New(404, "not_found", "That referral is not available.")
		}
		if err != nil {
			return err
		}
		report, err := workflow.LoadRouteReport(ctx, tx, issueID)
		if err != nil {
			return err
		}
		if report.Provenance != "USER_SELECTED_VALIDATED" && report.Provenance != "HUMAN_VERIFIED" {
			heldIssue = issueID
			held = reviewReason(report)
			return httpx.New(400, "validation_error", "A suggestion cannot start an escalation. Nothing was sent.")
		}
		var target uuid.UUID
		var targetName string
		err = tx.QueryRow(ctx, `
			SELECT s.target_authority_id, t.official_name
			FROM escalation_steps s JOIN authorities t ON t.id = s.target_authority_id
			WHERE s.authority_id = $1 ORDER BY s.stage LIMIT 1
		`, authorityID).Scan(&target, &targetName)
		if err == pgx.ErrNoRows {
			return httpx.New(400, "validation_error", "No verified escalation step is configured. Nothing was sent.")
		}
		if err != nil {
			return err
		}
		id := uuid.New()
		if _, err := tx.Exec(ctx, `
			INSERT INTO referrals (id, issue_id, authority_id, parent_referral_id, status, channel, packet)
			VALUES ($1, $2, $3, $4, 'drafted', 'manual', '{"escalation":true}'::jsonb)
		`, id, issueID, target, referralID); err != nil {
			return err
		}
		body := "Escalation from " + name + " was queued for " + targetName + ". A person must review it. No message was sent."
		if _, err := tx.Exec(ctx, `
			UPDATE referrals
			SET escalation_due_at = now() + interval '24 hours', next_actor = 'reviewer', updated_at = now()
			WHERE id = $1
		`, referralID); err != nil {
			return err
		}
		if err := addEvent(ctx, tx, id, principal.UserID, "drafted", body); err != nil {
			return err
		}
		if err := workflow.OpenReview(ctx, tx, issueID, body); err != nil {
			return err
		}
		return workflow.Record(ctx, tx, principal.UserID, issueID, reporter, "escalation", body, "Escalation queued")
	})
	if held != "" {
		_ = s.DB.WithFlags(ctx, &principal.UserID, []string{"app.platform_admin"}, func(tx pgx.Tx) error {
			return workflow.OpenReview(ctx, tx, heldIssue, held)
		})
	}
	return err
}
