package secretbox

import "testing"

func TestRoundTripAndLegacy(t *testing.T) {
	b, err := New("test-key", "jwt")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := b.Seal("s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if !IsSealed(sealed) || sealed == "s3cret" {
		t.Fatalf("expected sealed value, got %q", sealed)
	}
	got, err := b.Open(sealed)
	if err != nil || got != "s3cret" {
		t.Fatalf("open: %q %v", got, err)
	}
	legacy, err := b.Open("plain-token")
	if err != nil || legacy != "plain-token" {
		t.Fatalf("legacy: %q %v", legacy, err)
	}
	if empty, _ := b.Seal(""); empty != "" {
		t.Fatal("empty should stay empty")
	}
	other, _ := New("other-key", "jwt")
	if _, err := other.Open(sealed); err == nil {
		t.Fatal("different key should not open")
	}
}
