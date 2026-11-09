package results

import "testing"

func verdictFor(vs []Verdict, use string) Verdict {
	for _, v := range vs {
		if v.Use == use {
			return v
		}
	}
	return Verdict{}
}

func TestVerdicts(t *testing.T) {
	fast := Verdicts(500, 100, 10, 1, 5)
	for _, v := range fast {
		if v.Level != LevelGood {
			t.Errorf("fast line: %s = %s, want good", v.Use, v.Level)
		}
	}
	if n := verdictFor(fast, "1 GB download").Note; n != "16s" {
		t.Errorf("1 GB at 500 Mbps: %q, want 16s", n)
	}

	far := Verdicts(150, 60, 250, 3, 10)
	if v := verdictFor(far, "Gaming"); v.Level != LevelPoor || v.Note != "260 ms lag" {
		t.Errorf("far gaming = %+v", v)
	}
	if v := verdictFor(far, "Video calls"); v.Level != LevelOK {
		t.Errorf("far calls = %+v", v)
	}

	slow := Verdicts(6, -1, 40, 2, -1)
	if v := verdictFor(slow, "4K video"); v.Level != LevelPoor {
		t.Errorf("6 Mbps 4K = %+v", v)
	}
	if v := verdictFor(slow, "1 GB upload"); v.Use != "" {
		t.Error("no upload verdict without an upload measurement")
	}

	// Bufferbloat turns a good ping into a laggy one.
	if v := verdictFor(Verdicts(300, 50, 20, 1, 150), "Gaming"); v.Level != LevelPoor {
		t.Errorf("20 ms ping + 150 ms bloat should be poor for games: %+v", v)
	}
}

func TestFmtSeconds(t *testing.T) {
	tests := map[float64]string{0.2: "1s", 13: "13s", 240: "4m", 7560: "2.1h"}
	for s, want := range tests {
		if got := fmtSeconds(s); got != want {
			t.Errorf("fmtSeconds(%v) = %q, want %q", s, got, want)
		}
	}
}
