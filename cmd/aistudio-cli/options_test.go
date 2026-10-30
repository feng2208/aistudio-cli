package main

import (
	"testing"
	"time"

	"aistudio-cli/internal/aistudio"
)

func TestStringSlice(t *testing.T) {
	var s stringSlice
	if err := s.Set("auth1, auth2"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if len(s) != 2 || s[0] != "auth1" || s[1] != "auth2" {
		t.Fatalf("unexpected slice elements: %v", s)
	}
	if s.String() != "auth1,auth2" {
		t.Fatalf("unexpected string format: %s", s.String())
	}
}

func TestTimeDuration(t *testing.T) {
	var d timeDuration
	if err := d.Set("15m"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if d.Duration != 15*time.Minute {
		t.Fatalf("unexpected duration: %v", d.Duration)
	}
}

func TestEventCollectorNonStream(t *testing.T) {
	collector := newEventCollector(false, false)

	err := collector.Handle(aistudio.Event{
		Kind: aistudio.EventText,
		Part: aistudio.Part{Text: "Hello "},
	})
	if err != nil {
		t.Fatalf("handle text 1 failed: %v", err)
	}

	err = collector.Handle(aistudio.Event{
		Kind: aistudio.EventText,
		Part: aistudio.Part{Text: "World!"},
	})
	if err != nil {
		t.Fatalf("handle text 2 failed: %v", err)
	}

	if collector.textBuilder.String() != "Hello World!" {
		t.Fatalf("expected text 'Hello World!', got %q", collector.textBuilder.String())
	}
}

func TestResolvePrompt(t *testing.T) {
	p, err := resolvePrompt("direct prompt", "")
	if err != nil {
		t.Fatalf("resolvePrompt failed: %v", err)
	}
	if p != "direct prompt" {
		t.Fatalf("unexpected prompt: %s", p)
	}
}
