package events

import "testing"

func TestEventTypeStringsUnique(t *testing.T) {
	seen := make(map[EventType]bool)
	for _, et := range AllEventTypes {
		if seen[et] {
			t.Fatalf("duplicate EventType string: %s", et)
		}
		seen[et] = true
	}
	if len(AllEventTypes) != 12 {
		t.Fatalf("expected 12 EventTypes, got %d", len(AllEventTypes))
	}
}

func TestEventScopeValues(t *testing.T) {
	if EventScopeSession != "Session" {
		t.Fatalf("expected Session, got %s", EventScopeSession)
	}
	if EventScopeLink != "Link" {
		t.Fatalf("expected Link, got %s", EventScopeLink)
	}
	if EventScopeGlobal != "Global" {
		t.Fatalf("expected Global, got %s", EventScopeGlobal)
	}
}
