package notifications

// Allow reports whether an outbound message is permitted.
// WhatsApp and SMS require an explicit opt-in. There is no silent exception:
// a later legal review would have to add a purpose before any opted-out
// WhatsApp message can be sent. Email account-security notices stay allowed.
func Allow(channel, purpose string, prefs map[string]bool) bool {
	switch channel {
	case "whatsapp", "sms":
		return prefs[channel]
	case "email":
		if purpose == "security" {
			return true
		}
		return prefs["email"]
	case "in_app":
		if enabled, ok := prefs["in_app"]; ok {
			return enabled
		}
		return true
	default:
		return false
	}
}
