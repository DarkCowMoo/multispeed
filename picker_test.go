package main

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParsePick(t *testing.T) {
	choices := pickerChoices()
	tests := []struct {
		answer string
		want   []string
	}{
		{"", DefaultRegions},
		{"  ", DefaultRegions},
		{"all", choices},
		{"1", []string{"local"}},
		{"local, Tokyo new york", nil}, // "new" and "york" are separate words
		{"local,tokyo,newyork", []string{"local", "tokyo", "newyork"}},
		{"1 1 local", []string{"local"}},
	}
	for _, tt := range tests {
		got, err := parsePick(tt.answer, choices)
		if tt.want == nil {
			if err == nil {
				t.Errorf("parsePick(%q): expected error, got %v", tt.answer, got)
			}
			continue
		}
		if err != nil || !slices.Equal(got, tt.want) {
			t.Errorf("parsePick(%q) = %v, %v; want %v", tt.answer, got, err, tt.want)
		}
	}
	if _, err := parsePick("0", choices); err == nil {
		t.Error("0 should be out of range")
	}
	if _, err := parsePick("99", choices); err == nil {
		t.Error("99 should be out of range")
	}
}

func TestPickRegionsRetriesThenAccepts(t *testing.T) {
	var out strings.Builder
	o := options{k: 1, parallel: 1, duration: time.Second}
	spec, err := pickRegions(strings.NewReader("atlantis\n1 tokyo\n"), &out, o)
	if err != nil {
		t.Fatal(err)
	}
	if spec != "local,tokyo" {
		t.Errorf("spec = %q", spec)
	}
	if !strings.Contains(out.String(), "Try again") {
		t.Errorf("expected a retry prompt:\n%s", out.String())
	}
	if _, err := ParseRegions(spec); err != nil {
		t.Errorf("picked spec must parse: %v", err)
	}
}

func TestPickRegionsListsEveryChoice(t *testing.T) {
	var out strings.Builder
	o := options{k: 1, parallel: 1, duration: time.Second}
	if _, err := pickRegions(strings.NewReader("\n"), &out, o); err != nil {
		t.Fatal(err)
	}
	for _, key := range pickerChoices() {
		if !strings.Contains(out.String(), displayName(key)) {
			t.Errorf("picker does not show %q", key)
		}
	}
}

func TestPickRegionsEOF(t *testing.T) {
	var out strings.Builder
	if _, err := pickRegions(strings.NewReader(""), &out, options{k: 1, parallel: 1}); err == nil {
		t.Error("expected an error when input ends")
	}
}
