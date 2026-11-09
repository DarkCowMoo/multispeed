package results

import (
	"fmt"
	"math"
)

// Verdict levels: how well a connection suits a use.
const (
	LevelGood = "good"
	LevelOK   = "ok"
	LevelPoor = "poor"
)

// Verdict says how well a connection suits one everyday use.
type Verdict struct {
	Use   string `json:"use"`
	Level string `json:"level"` // good, ok or poor
	Note  string `json:"note,omitempty"`
}

// Verdicts judges a region's results for everyday uses. upMbps < 0 means
// upload was not measured; bloatMs < 0 means latency under load is unknown.
func Verdicts(downMbps, upMbps, pingMs, jitterMs, bloatMs float64) []Verdict {
	var out []Verdict

	video := Verdict{Use: "4K video", Level: LevelPoor, Note: "SD only"}
	switch {
	case downMbps >= 25:
		video = Verdict{Use: "4K video", Level: LevelGood}
	case downMbps >= 8:
		video = Verdict{Use: "4K video", Level: LevelOK, Note: "HD only"}
	}
	out = append(out, video)

	call := Verdict{Use: "Video calls", Level: LevelGood}
	switch {
	case downMbps < 2 || (upMbps >= 0 && upMbps < 1.5) || pingMs >= 300 || jitterMs >= 50:
		call.Level = LevelPoor
	case downMbps < 4 || (upMbps >= 0 && upMbps < 3) || pingMs >= 150 || jitterMs >= 30:
		call.Level = LevelOK
	}
	if call.Level != LevelGood && pingMs >= 150 {
		call.Note = "noticeable delay"
	}
	out = append(out, call)

	// For games, what matters is latency while others use the line too.
	effective := pingMs
	if bloatMs > 0 {
		effective += bloatMs
	}
	game := Verdict{Use: "Gaming", Level: LevelGood}
	switch {
	case effective >= 120:
		game = Verdict{Use: "Gaming", Level: LevelPoor, Note: fmt.Sprintf("%.0f ms lag", effective)}
	case effective >= 60:
		game = Verdict{Use: "Gaming", Level: LevelOK, Note: fmt.Sprintf("%.0f ms lag", effective)}
	}
	out = append(out, game)

	big := Verdict{Use: "1 GB download", Level: LevelPoor}
	if downMbps > 0 {
		big.Note = fmtSeconds(8000 / downMbps) // 1 GB = 8000 Mbit
	}
	switch {
	case downMbps >= 200:
		big.Level = LevelGood
	case downMbps >= 50:
		big.Level = LevelOK
	}
	out = append(out, big)

	if upMbps >= 0 {
		up := Verdict{Use: "1 GB upload", Level: LevelPoor}
		if upMbps > 0 {
			up.Note = fmtSeconds(8000 / upMbps)
		}
		switch {
		case upMbps >= 100:
			up.Level = LevelGood
		case upMbps >= 20:
			up.Level = LevelOK
		}
		out = append(out, up)
	}
	return out
}

// fmtSeconds formats a duration in seconds as "13s", "4m" or "2.1h".
func fmtSeconds(s float64) string {
	switch {
	case s < 90:
		return fmt.Sprintf("%.0fs", math.Max(1, s))
	case s < 90*60:
		return fmt.Sprintf("%.0fm", s/60)
	default:
		return fmt.Sprintf("%.1fh", s/3600)
	}
}
