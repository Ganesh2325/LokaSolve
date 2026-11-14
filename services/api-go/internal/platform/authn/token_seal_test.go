package authn

import "testing"

func TestSealRoundTrip(t *testing.T) {
	tokens := NewTokenManager("test-secret", 0, 0)
	sealed, err := tokens.Seal("refresh-token-value")
	if err != nil {
		t.Fatal(err)
	}
	if sealed == "refresh-token-value" {
		t.Fatal("seal stored the raw token")
	}
	opened, err := tokens.Open(sealed)
	if err != nil || opened != "refresh-token-value" {
		t.Fatalf("opened %q err %v", opened, err)
	}
	other := NewTokenManager("other-secret", 0, 0)
	if _, err := other.Open(sealed); err == nil {
		t.Fatal("another secret opened the seal")
	}
}
