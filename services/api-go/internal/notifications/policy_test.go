package notifications

import "testing"

func TestWhatsAppOptOutBlocksLaterMessages(t *testing.T) {
	prefs := map[string]bool{"in_app": true, "email": true, "sms": false, "whatsapp": false}
	if Allow("whatsapp", "status_update", prefs) {
		t.Fatal("default whatsapp preference allowed an outbound message")
	}
	prefs["whatsapp"] = true
	if !Allow("whatsapp", "status_update", prefs) {
		t.Fatal("explicit opt-in was ignored")
	}
	prefs["whatsapp"] = false
	if Allow("whatsapp", "security", prefs) {
		t.Fatal("opt-out still allowed a whatsapp message")
	}
	if !Allow("email", "security", map[string]bool{"email": false}) {
		t.Fatal("account security email should remain available")
	}
}
