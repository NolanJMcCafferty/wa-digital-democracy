package connector

import "testing"

func TestRegisterAndLookup(t *testing.T) {
	// Use unique systems so we don't collide with real connectors when
	// the test binary imports them.
	d := Descriptor{System: "test-connector-a", BaseURL: "https://example.test", RateLimitHz: 5}
	Register(d)

	got, ok := Lookup("test-connector-a")
	if !ok {
		t.Fatal("Lookup returned !ok for registered system")
	}
	if got != d {
		t.Fatalf("Lookup returned %+v, want %+v", got, d)
	}

	found := false
	for _, r := range Registered() {
		if r.System == "test-connector-a" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Registered() did not return test-connector-a")
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	Register(Descriptor{System: "test-connector-dup"})
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate Register")
		}
	}()
	Register(Descriptor{System: "test-connector-dup"})
}

func TestRegisterEmptySystemPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on empty System")
		}
	}()
	Register(Descriptor{})
}

func TestAcceptHelpers(t *testing.T) {
	if got := JSONAccept().Get("Accept"); got != "application/json" {
		t.Errorf("JSONAccept Accept = %q", got)
	}
	if got := HTMLAccept().Get("Accept"); got != "text/html" {
		t.Errorf("HTMLAccept Accept = %q", got)
	}
}
