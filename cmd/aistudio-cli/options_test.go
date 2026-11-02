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

func TestResolveWAABackend(t *testing.T) {
	tests := []struct {
		name    string
		flag    string
		env     string
		want    string
		wantErr bool
	}{
		{name: "default camoufox", want: waaBackendCamoufox},
		{name: "env spidermonkey", env: "spidermonkey", want: waaBackendSpiderMonkey},
		{name: "env trims and lowercases", env: " SpiderMonkey ", want: waaBackendSpiderMonkey},
		{name: "flag overrides env", flag: "spidermonkey", env: "camoufox", want: waaBackendSpiderMonkey},
		{name: "flag camoufox", flag: "camoufox", want: waaBackendCamoufox},
		{name: "invalid flag", flag: "native", wantErr: true},
		{name: "invalid flag overrides env", flag: "native", env: "camoufox", wantErr: true},
		{name: "invalid env", env: "native", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("WAA_BACKEND", tt.env)
			got, err := resolveWAABackend(tt.flag)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got backend %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveWAABackend failed: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected backend %q, got %q", tt.want, got)
			}
		})
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
