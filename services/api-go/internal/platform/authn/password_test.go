package authn

import "testing"

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := VerifyPassword(hash, "correct horse battery")
	if err != nil || !ok {
		t.Fatalf("expected match, ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword(hash, "wrong horse battery")
	if err != nil || ok {
		t.Fatalf("expected mismatch, ok=%v err=%v", ok, err)
	}
	ok, _ = VerifyPassword("not-a-hash", "correct horse battery")
	if ok {
		t.Fatal("malformed hash matched")
	}
}
