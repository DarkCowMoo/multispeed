package regions

import (
	"strings"
	"testing"
)

func TestParseRegions(t *testing.T) {
	rs, err := ParseRegions("local, Frankfurt, New York, Lisbon=38.72,-9.14,tokyo")
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 5 {
		t.Fatalf("want 5 regions, got %d: %+v", len(rs), rs)
	}
	if !rs[0].Local || rs[1].Name != "Frankfurt" || rs[2].Name != "New York" {
		t.Errorf("unexpected: %+v", rs[:3])
	}
	if rs[3].Name != "Lisbon" || rs[3].Lat != 38.72 || rs[3].Lon != -9.14 {
		t.Errorf("custom region parsed wrong: %+v", rs[3])
	}
	if rs[4].Name != "Tokyo" {
		t.Errorf("want Tokyo, got %+v", rs[4])
	}
}

func TestParseRegionsErrors(t *testing.T) {
	tests := map[string]string{
		"unknown region": "atlantis",
		"bad latitude":   "X=100,0",
		"bad longitude":  "X=0,200",
		"missing lon":    "X=1",
		"empty":          " , ",
	}
	for name, spec := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseRegions(spec); err == nil {
				t.Errorf("ParseRegions(%q): expected error", spec)
			}
		})
	}
}

func TestDefaultRegionsParse(t *testing.T) {
	if _, err := ParseRegions(strings.Join(DefaultRegions, ",")); err != nil {
		t.Errorf("default regions must parse: %v", err)
	}
}
