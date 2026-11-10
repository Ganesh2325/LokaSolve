package workflow

import "strings"
import "testing"

func TestAssessStreetLight(t *testing.T) {
	got := Assess("The streetlight on lane 4 has been dark for three nights.")
	if got.Suggestion != "Lighting" || got.Confidence <= 0 || !got.NeedsReview {
		t.Fatalf("unexpected assessment: %+v", got)
	}
}

func TestAssessUnknownGoesToReview(t *testing.T) {
	got := Assess("Something odd is happening beside the school gate.")
	if got.Suggestion != "" || !got.NeedsReview {
		t.Fatalf("expected a review with no label, got %+v", got)
	}
}

func TestAssessDoesNotInventADispatch(t *testing.T) {
	got := Assess("There is a gas leak outside the shop.")
	if !strings.Contains(got.Safety, "has not dispatched") {
		t.Fatalf("safety guidance missing: %+v", got)
	}
}
