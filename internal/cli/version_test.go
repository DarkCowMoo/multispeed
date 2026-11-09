package cli

import (
	"testing"
	"time"
)

func TestReleaseVersion(t *testing.T) {
	tests := map[string]string{
		"":        "dev",
		"(devel)": "dev",
		"v1.2.0":  "v1.2.0",
		"v0.0.0-20261109153144-bb496671b87e+dirty": "dev",
		"v0.0.0-20261109153144-bb496671b87e":       "dev",
		"v1.2.1-0.20261109153144-bb496671b87e":     "dev",
	}
	for in, want := range tests {
		if got := releaseVersion(in); got != want {
			t.Errorf("releaseVersion(%q) = %q, want %q", in, got, want)
		}
	}
	if got := version("v9.9.9"); got != "v9.9.9" {
		t.Errorf("a release build's version wins, got %q", got)
	}
}

func TestPlanSummary(t *testing.T) {
	o := options{k: 1, parallel: 1, duration: 10 * time.Second, noUpload: true}
	if got := planSummary(3, o); got != "3 regions × 1 server, download only, about 48s" {
		t.Errorf("plan = %q", got)
	}
}
