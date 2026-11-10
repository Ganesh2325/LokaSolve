package workflow

import "regexp"

type Assessment struct {
	Suggestion  string
	Confidence  float64
	NeedsReview bool
	Reason      string
	Safety      string
}

var labelRules = []struct {
	pattern *regexp.Regexp
	label   string
}{
	{regexp.MustCompile(`(?i)street\s*light|streetlight|\blight\b`), "Lighting"},
	{regexp.MustCompile(`(?i)pothole|\broad\b|footpath|pavement`), "Roads"},
	{regexp.MustCompile(`(?i)\bwater\b|leak|pipe|\btap\b`), "Water"},
	{regexp.MustCompile(`(?i)drain|sewage|gutter`), "Drainage"},
	{regexp.MustCompile(`(?i)\bpower\b|electric|transformer|outage`), "Power"},
	{regexp.MustCompile(`(?i)garbage|trash|\bwaste\b`), "Waste"},
}

var emergencyPattern = regexp.MustCompile(`(?i)\b(gas leak|electrocution|electric shock|live wire|explosion|not breathing|unconscious|heart attack|building fire|house fire)\b`)
var crisisPattern = regexp.MustCompile(`(?i)(\bsuicid|\bkill myself\b|\bself[- ]harm\b|\bwant to die\b)`)

func Assess(text string) Assessment {
	result := Assessment{NeedsReview: true, Reason: "No keyword rule matched. A person should read this report."}
	for _, rule := range labelRules {
		if rule.pattern.MatchString(text) {
			result.Suggestion = rule.label
			result.Confidence = 0.62
			result.Reason = "A keyword rule suggested " + rule.label + ". This is not an official classification."
			break
		}
	}
	if emergencyPattern.MatchString(text) {
		result.Safety = "This may be an immediate danger. Call 112. LokaSolve has not dispatched anyone."
	} else if crisisPattern.MatchString(text) {
		result.Safety = "If this is a mental-health crisis, Tele-MANAS is 14416 and the emergency number is 112. LokaSolve has not dispatched anyone."
	}
	return result
}
