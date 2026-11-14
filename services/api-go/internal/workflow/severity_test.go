package workflow

import (
	"regexp"
	"testing"
)

func TestMatchRuleOrdersSeverity(t *testing.T) {
	rules := []Rule{
		{ID: "p0", Severity: "P0", Pattern: `(?i)live wire|building fire|structural collapse`},
		{ID: "p1", Severity: "P1", Pattern: `(?i)contamination|power outage`},
		{ID: "p2", Severity: "P2", Pattern: `(?i)water disruption|major road|drainage failure`},
		{ID: "p3", Severity: "P3", Pattern: `(?i)street\s*light|streetlight|garbage`},
		{ID: "p4", Severity: "P4", Pattern: `(?i)plumbing|appliance|electrician|mechanic`},
	}
	cases := []struct {
		text string
		want string
	}{
		{"Exposed live wire in the lane", "P0"},
		{"Severe water contamination at the tank", "P1"},
		{"Major road damage across the ward", "P2"},
		{"Street light out on the corner", "P3"},
		{"Need a private electrician for the flat", "P4"},
		{"The courtyard tap drips", ""},
	}
	for _, tc := range cases {
		got, ok := MatchRule(rules, tc.text)
		if tc.want == "" {
			if ok {
				t.Fatalf("%q matched %s", tc.text, got.Severity)
			}
			continue
		}
		if !ok || got.Severity != tc.want {
			t.Fatalf("%q got %q want %s", tc.text, got.Severity, tc.want)
		}
	}
	if _, err := regexp.Compile(rules[0].Pattern); err != nil {
		t.Fatal(err)
	}
}
