package workflow

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// RouteReport is the evidence available before an authority or provider action.
// A validated category is not proof that a government office has jurisdiction.
type RouteReport struct {
	Provenance         string
	Category           string
	Suggestion         string
	Locality           string
	Severity           string
	HasCoordinates     bool
	JurisdictionOnFile bool
	HumanApproved      bool
}

// RouteTarget is one office or provider being considered.
type RouteTarget struct {
	CategoryMatch bool
	LocalityMatch bool
	Verified      bool
	ChannelReady  bool
}

// RouteDecision separates a recommendation shown to a person from an action
// the service is willing to record.
type RouteDecision struct {
	Recommend bool
	Allow     bool
	Reasons   []string
}

func suitableClassification(provenance string) bool {
	return provenance == "USER_SELECTED_VALIDATED" || provenance == "HUMAN_VERIFIED"
}

func humanApprovalRequired(severity string) bool {
	switch severity {
	case "P3", "P4":
		return false
	default:
		return true
	}
}

// DecideRoute allows a routing action only when classification, location,
// jurisdiction, a verified responsibility mapping, an authorized channel, and
// any required human approval are all present. A keyword or model suggestion
// can be shown to a reviewer and cannot authorize the action.
func DecideRoute(report RouteReport, target RouteTarget) RouteDecision {
	decision := RouteDecision{}
	if report.Suggestion != "" && !suitableClassification(report.Provenance) {
		decision.Reasons = append(decision.Reasons, "The label "+report.Suggestion+" is a suggestion and is not an authority routing instruction.")
	}
	if !suitableClassification(report.Provenance) || strings.TrimSpace(report.Category) == "" {
		decision.Reasons = append(decision.Reasons, "No verified classification is available. The report stays in human review.")
		decision.Reasons = append(decision.Reasons, "Nothing was sent.")
		return decision
	}
	decision.Reasons = append(decision.Reasons, "A validated category is not proof of government jurisdiction.")
	if !target.CategoryMatch {
		decision.Reasons = append(decision.Reasons, "This destination does not list the validated category.")
		decision.Reasons = append(decision.Reasons, "Nothing was sent.")
		return decision
	}
	decision.Recommend = true
	decision.Reasons = append(decision.Reasons, "This is a routing recommendation. No complaint has been submitted and no provider has been dispatched.")
	if !report.HasCoordinates {
		decision.Reasons = append(decision.Reasons, "Location is not validated because coordinates were not stored.")
	}
	if !report.JurisdictionOnFile || !target.LocalityMatch {
		decision.Reasons = append(decision.Reasons, "Jurisdiction does not match, so automatic routing stops.")
	}
	if !target.Verified {
		decision.Reasons = append(decision.Reasons, "The destination is not verified.")
	}
	if !target.ChannelReady {
		decision.Reasons = append(decision.Reasons, "No authorized communication channel is on file.")
	}
	if humanApprovalRequired(report.Severity) && !report.HumanApproved {
		decision.Reasons = append(decision.Reasons, "This severity still needs a person to approve routing.")
	}
	decision.Allow = report.HasCoordinates && report.JurisdictionOnFile && target.LocalityMatch && target.Verified && target.ChannelReady && (!humanApprovalRequired(report.Severity) || report.HumanApproved)
	if !decision.Allow {
		decision.Reasons = append(decision.Reasons, "The report stays in human review. Nothing was sent.")
	}
	return decision
}

// LoadRouteReport reads the projected report. Missing provenance is unclassified.
func LoadRouteReport(ctx context.Context, tx pgx.Tx, issueID uuid.UUID) (RouteReport, error) {
	var report RouteReport
	var locality, category, suggestion *string
	var lat, lng *float64
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(classification_provenance, 'UNCLASSIFIED'), category_label, suggested_label,
			locality_text, latitude, longitude, COALESCE(severity, '')
		FROM issue_visible($1)
	`, issueID).Scan(&report.Provenance, &category, &suggestion, &locality, &lat, &lng, &report.Severity)
	if err != nil {
		return RouteReport{}, err
	}
	if category != nil {
		report.Category = strings.TrimSpace(*category)
	}
	if suggestion != nil {
		report.Suggestion = strings.TrimSpace(*suggestion)
	}
	if locality != nil {
		report.Locality = strings.TrimSpace(*locality)
	}
	report.HasCoordinates = lat != nil && lng != nil
	if report.Locality != "" {
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM organization_jurisdictions
				WHERE lower(btrim(locality_text)) = lower(btrim($1))
			)
		`, report.Locality).Scan(&report.JurisdictionOnFile); err != nil {
			return RouteReport{}, err
		}
	}
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM review_items
			WHERE issue_id = $1 AND decision IN ('confirm_routing', 'correct_routing')
		)
	`, issueID).Scan(&report.HumanApproved); err != nil {
		return RouteReport{}, err
	}
	if !suitableClassification(report.Provenance) {
		report.Category = ""
	}
	return report, nil
}

func SamePlace(left, right string) bool {
	left = strings.ToLower(strings.TrimSpace(left))
	right = strings.ToLower(strings.TrimSpace(right))
	if left == "" || right == "" {
		return false
	}
	return strings.Contains(left, right) || strings.Contains(right, left)
}

func providerTarget(item Resolver, report RouteReport) RouteTarget {
	locality := ""
	if item.LocalityText != nil {
		locality = *item.LocalityText
	}
	reviewed := item.VerificationStatus == "reviewed"
	return RouteTarget{
		CategoryMatch: CategoryListed(item.Skills, report.Category),
		LocalityMatch: SamePlace(report.Locality, locality),
		Verified:      reviewed,
		ChannelReady:  reviewed,
	}
}

func providerReviewReason(report RouteReport) string {
	if report.Suggestion != "" {
		return "Suggested department: " + report.Suggestion + ". A provider was not offered and no external submission was made."
	}
	return "A provider was not offered. The report stays in human review. Nothing was sent."
}

func CategoryListed(categories, category string) bool {
	category = strings.ToLower(strings.TrimSpace(category))
	if category == "" {
		return false
	}
	return strings.Contains(strings.ToLower(categories), category)
}
