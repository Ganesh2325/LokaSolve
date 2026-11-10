package authority

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"lokasolve/services/api-go/internal/identity"
	"lokasolve/services/api-go/internal/platform/audit"
	"lokasolve/services/api-go/internal/platform/authn"
	"lokasolve/services/api-go/internal/platform/database"
	"lokasolve/services/api-go/internal/platform/httpx"
	"lokasolve/services/api-go/internal/workflow"
)

type Service struct {
	DB *database.DB
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
		var category, locality *string
		err := tx.QueryRow(ctx, `SELECT category_label, locality_text FROM issues WHERE id = $1`, issueID).Scan(&category, &locality)
		if err == pgx.ErrNoRows {
			return httpx.New(404, "not_found", "That report is not available.")
		}
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id::text, official_name, org_type, locality_text, categories, verification_status, verification_source FROM authorities ORDER BY official_name`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item Office
			if err := rows.Scan(&item.ID, &item.OfficialName, &item.OrgType, &item.LocalityText, &item.Categories, &item.VerificationStatus, &item.VerificationSource); err != nil {
				return err
			}
			if explanation, ok := officeMatch(item, category, locality); ok {
				item.Explanation = explanation
				item.VerificationSource = nil
				found = append(found, item)
			}
		}
		return rows.Err()
	})
	return found, err
}

func officeMatch(item Office, category, locality *string) (string, bool) {
	reasons := []string{}
	if category != nil && *category != "" && strings.Contains(strings.ToLower(item.Categories), strings.ToLower(*category)) {
		reasons = append(reasons, "covers "+*category)
	}
	if locality != nil && item.LocalityText != nil && *locality != "" && *item.LocalityText != "" &&
		(strings.Contains(strings.ToLower(*locality), strings.ToLower(*item.LocalityText)) || strings.Contains(strings.ToLower(*item.LocalityText), strings.ToLower(*locality))) {
		reasons = append(reasons, "lists "+*item.LocalityText)
	}
	if len(reasons) == 0 {
		return "", false
	}
	note := "Matched because it " + strings.Join(reasons, " and ") + ". Verification is " + item.VerificationStatus + ". No message has been sent."
	return note, true
}

func (s *Service) Prepare(ctx context.Context, principal authn.Principal, issueID, authorityID uuid.UUID, consent bool) (Referral, error) {
	if !consent {
		return Referral{}, httpx.New(400, "validation_error", "Consent is required before a complaint packet is prepared.")
	}
	var created Referral
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		var description string
		var category, locality, address *string
		var share bool
		var reporter uuid.UUID
		err := tx.QueryRow(ctx, `
			SELECT description, category_label, locality_text, address_text, identity_visible_to_resolver, reporter_user_id
			FROM issues WHERE id = $1
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
			INSERT INTO referrals (id, issue_id, authority_id, status, channel, consent_at, packet)
			VALUES ($1, $2, $3, 'consented', $4, $5, $6::jsonb)
		`, id, issueID, authorityID, channel, now, raw); err != nil {
			return err
		}
		if err := addEvent(ctx, tx, id, principal.UserID, "consented", "A complaint packet was prepared with consent. It has not been sent."); err != nil {
			return err
		}
		created = Referral{ID: id.String(), IssueID: issueID.String(), AuthorityID: authorityID.String(), AuthorityName: name, Status: "consented", Channel: channel, ConsentAt: &now, CreatedAt: now}
		return workflow.Record(ctx, tx, principal.UserID, issueID, reporter, "referral", "A complaint packet for "+name+" was prepared. Verification is "+verification+". Nothing was sent.", "Packet prepared")
	})
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
			FROM referrals r JOIN issues i ON i.id = r.issue_id WHERE r.id = $1
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
		if (next == "acknowledged" || next == "rejected" || next == "failed" || next == "disputed") && note == "" {
			return httpx.Validation(map[string]string{"note": "Write what happened."})
		}
		body := "Referral status is " + strings.ReplaceAll(next, "_", " ") + "."
		if note != "" {
			body += " " + note
		}
		if next == "manual_handoff" || next == "not_supported" {
			body = "No email, WhatsApp, or SMS provider is connected. The packet is ready for a manual handoff on the " + channel + " channel. Nothing was delivered."
		}
		var ref *string
		if reference != "" {
			ref = &reference
		}
		if _, err := tx.Exec(ctx, `
			UPDATE referrals SET status = $2, external_reference = COALESCE($3, external_reference), updated_at = now() WHERE id = $1
		`, referralID, next, ref); err != nil {
			return err
		}
		if err := addEvent(ctx, tx, referralID, principal.UserID, next, body); err != nil {
			return err
		}
		return workflow.Record(ctx, tx, principal.UserID, issueID, reporter, "referral", body, "Referral updated")
	})
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
	return s.DB.WithFlags(ctx, &principal.UserID, []string{"app.platform_admin"}, func(tx pgx.Tx) error {
		var authorityID, issueID, reporter uuid.UUID
		var name string
		err := tx.QueryRow(ctx, `
			SELECT r.authority_id, r.issue_id, i.reporter_user_id, a.official_name
			FROM referrals r
			JOIN issues i ON i.id = r.issue_id
			JOIN authorities a ON a.id = r.authority_id
			WHERE r.id = $1
		`, referralID).Scan(&authorityID, &issueID, &reporter, &name)
		if err == pgx.ErrNoRows {
			return httpx.New(404, "not_found", "That referral is not available.")
		}
		if err != nil {
			return err
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
		if err := addEvent(ctx, tx, id, principal.UserID, "drafted", body); err != nil {
			return err
		}
		return workflow.Record(ctx, tx, principal.UserID, issueID, reporter, "escalation", body, "Escalation queued")
	})
}
