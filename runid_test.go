package main

import (
	"regexp"
	"testing"
)

func TestNewRunID(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{4}-[0-9a-f]{4}$`)
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := newRunID()
		if !re.MatchString(id) {
			t.Fatalf("run ID %q has the wrong shape", id)
		}
		if seen[id] {
			t.Fatalf("duplicate run ID %q", id)
		}
		seen[id] = true
	}
}
