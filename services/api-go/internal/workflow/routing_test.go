package workflow

import "testing"

func TestKeywordSuggestionCannotAuthorizeRouting(t *testing.T) {
	decision := DecideRoute(RouteReport{
		Provenance: "RULE_SUGGESTED", Suggestion: "Power", Category: "Power",
		Locality: "Market", HasCoordinates: true, JurisdictionOnFile: true, HumanApproved: true, Severity: "P3",
	}, RouteTarget{CategoryMatch: true, LocalityMatch: true, Verified: true, ChannelReady: true})
	if decision.Allow || decision.Recommend {
		t.Fatalf("suggestion authorized routing: %+v", decision)
	}
}

func TestAISuggestionCannotAuthorizeRouting(t *testing.T) {
	decision := DecideRoute(RouteReport{
		Provenance: "AI_SUGGESTED", Suggestion: "Lighting", Severity: "P4", HumanApproved: true,
		HasCoordinates: true, JurisdictionOnFile: true,
	}, RouteTarget{CategoryMatch: true, LocalityMatch: true, Verified: true, ChannelReady: true})
	if decision.Allow || decision.Recommend {
		t.Fatalf("model suggestion authorized routing: %+v", decision)
	}
}

func TestValidatedCategoryCanRecommendWithoutJurisdiction(t *testing.T) {
	decision := DecideRoute(RouteReport{
		Provenance: "USER_SELECTED_VALIDATED", Category: "Lighting",
		Locality: "Market", HasCoordinates: true, Severity: "P3",
	}, RouteTarget{CategoryMatch: true, Verified: true, ChannelReady: true})
	if !decision.Recommend || decision.Allow {
		t.Fatalf("recommendation = %+v", decision)
	}
}

func TestIncorrectJurisdictionBlocksTheAction(t *testing.T) {
	decision := DecideRoute(RouteReport{
		Provenance: "HUMAN_VERIFIED", Category: "Drainage",
		HasCoordinates: true, JurisdictionOnFile: false, HumanApproved: true, Severity: "P1",
	}, RouteTarget{CategoryMatch: true, LocalityMatch: false, Verified: true, ChannelReady: true})
	if decision.Allow {
		t.Fatalf("jurisdiction mismatch was allowed: %+v", decision)
	}
}

func TestCompleteEvidenceCanRecordAnAction(t *testing.T) {
	decision := DecideRoute(RouteReport{
		Provenance: "HUMAN_VERIFIED", Category: "Lighting", Locality: "Market",
		HasCoordinates: true, JurisdictionOnFile: true, HumanApproved: true, Severity: "P1",
	}, RouteTarget{CategoryMatch: true, LocalityMatch: true, Verified: true, ChannelReady: true})
	if !decision.Allow || !decision.Recommend {
		t.Fatalf("complete evidence was blocked: %+v", decision)
	}
}

func TestHighSeverityStillNeedsAPerson(t *testing.T) {
	decision := DecideRoute(RouteReport{
		Provenance: "USER_SELECTED_VALIDATED", Category: "Water",
		HasCoordinates: true, JurisdictionOnFile: true, Severity: "P0",
	}, RouteTarget{CategoryMatch: true, LocalityMatch: true, Verified: true, ChannelReady: true})
	if decision.Allow || !decision.Recommend {
		t.Fatalf("unapproved severe route = %+v", decision)
	}
}
