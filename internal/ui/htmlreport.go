package ui

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"strings"

	"github.com/DarkCowMoo/multispeed/internal/measure"
	"github.com/DarkCowMoo/multispeed/internal/results"
)

// htmlData is what the HTML report template shows.
type htmlData struct {
	R       *results.Report
	Best    float64
	Map     string
	Trends  map[string][]float64
	RawJSON string
}

// WriteHTML saves the report as one self-contained HTML page.
func WriteHTML(path string, r *results.Report, hist []results.HistoryEntry) error {
	d := htmlData{R: r, Trends: map[string][]float64{}}
	for i := range r.Regions {
		d.Best = max(d.Best, r.Regions[i].BestDownMbps)
	}
	var m strings.Builder
	if err := PrintMap(&m, r.Regions, Palette{}); err == nil {
		d.Map = m.String()
	}
	for _, s := range results.SeriesByRegion(hist) {
		if len(s.Downs) >= 2 {
			d.Trends[s.Region] = lastN(s.Downs, sparkWidth)
		}
	}
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	d.RawJSON = string(raw)

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := htmlTemplate.Execute(f, d); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// sparkPoints turns values into SVG polyline points in a w×h box.
func sparkPoints(v []float64, w, h float64) string {
	if len(v) < 2 {
		return ""
	}
	lo, hi := results.MinMax(v)
	pts := make([]string, len(v))
	for i, x := range v {
		y := h / 2
		if hi > lo {
			y = h - 2 - (x-lo)/(hi-lo)*(h-4)
		}
		pts[i] = fmt.Sprintf("%.1f,%.1f", float64(i)/float64(len(v)-1)*w, y)
	}
	return strings.Join(pts, " ")
}

var htmlTemplate = template.Must(template.New("report").Funcs(template.FuncMap{
	"pct": func(v, best float64) float64 {
		if best <= 0 {
			return 0
		}
		return v / best * 100
	},
	"gradeClass": func(g string) string {
		switch g {
		case "A+", "A":
			return "good"
		case "B", "C":
			return "fair"
		case "":
			return "dim"
		}
		return "bad"
	},
	"levelClass": func(l string) string {
		switch l {
		case results.LevelGood:
			return "good"
		case results.LevelOK:
			return "fair"
		}
		return "bad"
	},
	"pingClass": func(ms float64) string {
		switch {
		case ms < 50:
			return "good"
		case ms < 150:
			return "fair"
		}
		return "bad"
	},
	"mark":   verdictMark,
	"spark":  sparkPoints,
	"date":   func(r *results.Report) string { return r.Time.Local().Format("Monday 2 January 2006, 15:04") },
	"bytes":  FmtBytes,
	"change": func(p *float64) string { t, _ := changeText(p, Palette{}); return t },
	"changeClass": func(p *float64) string {
		switch {
		case p == nil:
			return "dim"
		case *p >= 5:
			return "good"
		case *p <= -5:
			return "bad"
		}
		return "dim"
	},
	"where": func(r *measure.Route) string { return r.JumpWhere() },
}).Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>multispeed report</title>
<style>
:root { --bg:#fbfbfa; --fg:#1d1d1f; --muted:#6b6b70; --line:#e4e4e2; --card:#fff;
  --accent:#0a84c6; --good:#1f8a4c; --fair:#b07800; --bad:#c4372f; }
@media (prefers-color-scheme: dark) { :root { --bg:#121214; --fg:#ececee; --muted:#9a9aa0;
  --line:#2a2a2e; --card:#1b1b1e; --accent:#4cb3e8; --good:#4cc47e; --fair:#e0a93a; --bad:#ef6b62; } }
* { box-sizing: border-box; }
body { margin:0; background:var(--bg); color:var(--fg);
  font:15px/1.5 system-ui,-apple-system,"Segoe UI",sans-serif; }
main { max-width:1100px; margin:0 auto; padding:32px 16px 64px; }
h1 { font-size:26px; margin:0 0 4px; }
h2 { font-size:17px; margin:36px 0 12px; }
.sub { color:var(--muted); margin:0; }
.card { background:var(--card); border:1px solid var(--line); border-radius:10px; overflow-x:auto; }
table { border-collapse:collapse; width:100%; }
th, td { padding:9px 12px; text-align:left; white-space:nowrap; border-bottom:1px solid var(--line); }
tr:last-child td { border-bottom:0; }
th { font-size:12px; text-transform:uppercase; letter-spacing:.04em; color:var(--muted); font-weight:600; }
td.num, th.num { text-align:right; font-variant-numeric:tabular-nums; }
.bar { width:140px; height:8px; background:var(--line); border-radius:4px; overflow:hidden; }
.bar > div { height:100%; background:var(--accent); }
.good { color:var(--good); } .fair { color:var(--fair); } .bad { color:var(--bad); } .dim { color:var(--muted); }
.grade { font-weight:700; }
pre { margin:0; padding:16px; font:12px/1.25 ui-monospace,"Cascadia Mono",Consolas,monospace; }
svg polyline { fill:none; stroke:var(--accent); stroke-width:1.5; }
details { margin-top:36px; } summary { cursor:pointer; color:var(--muted); }
footer { margin-top:40px; color:var(--muted); font-size:13px; }
</style>
</head>
<body>
<main>
<h1>Internet speed by region</h1>
<p class="sub">{{date .R}}{{with .R.ISP}} · {{.}}{{end}} · {{len .R.Regions}} regions · {{bytes .R.DataBytes}} used · run {{.R.RunID}}</p>

<h2>Results</h2>
<div class="card"><table>
<tr><th>Region</th><th class="num">Ping</th><th class="num">Jitter</th><th class="num">Download</th><th></th>
{{if .R.Settings.Upload}}<th class="num">Upload</th>{{end}}<th>Bufferbloat</th><th class="num">vs last</th><th>Trend</th><th>Best server</th></tr>
{{range .R.Regions}}{{if gt .OK 0}}
<tr><td><b>{{.Region}}</b></td>
<td class="num {{pingClass .PingMs}}">{{printf "%.0f" .PingMs}} ms</td>
<td class="num">{{printf "%.1f" .JitterMs}} ms</td>
<td class="num"><b>{{printf "%.1f" .BestDownMbps}}</b> Mbps</td>
<td><div class="bar"><div style="width:{{printf "%.0f" (pct .BestDownMbps $.Best)}}%"></div></div></td>
{{if $.R.Settings.Upload}}<td class="num">{{printf "%.1f" .BestUpMbps}} Mbps</td>{{end}}
<td><span class="grade {{gradeClass .Grade}}">{{or .Grade "–"}}</span>{{if .Grade}} <span class="dim">+{{printf "%.0f" .BloatMs}} ms</span>{{end}}</td>
<td class="num {{changeClass .ChangePct}}">{{change .ChangePct}}</td>
<td>{{with index $.Trends .Region}}<svg width="90" height="22" viewBox="0 0 90 22"><polyline points="{{spark . 90 22}}"/></svg>{{end}}</td>
<td class="dim">{{.BestServer}}</td></tr>
{{else}}
<tr><td><b>{{.Region}}</b></td><td colspan="9" class="bad">No result ({{.Note}})</td></tr>
{{end}}{{end}}
</table></div>

<h2>What each region is good for</h2>
<div class="card"><table>
{{range .R.Regions}}{{if .GoodFor}}
<tr><td><b>{{.Region}}</b></td>{{range .GoodFor}}<td class="{{levelClass .Level}}">{{mark .Level}} {{.Use}}{{with .Note}} <span class="dim">{{.}}</span>{{end}}</td>{{end}}</tr>
{{end}}{{end}}
</table></div>

{{if .Map}}<h2>Map</h2>
<div class="card"><pre>{{.Map}}</pre></div>{{end}}

{{$routes := false}}{{range .R.Regions}}{{if .Route}}{{$routes = true}}{{end}}{{end}}
{{if $routes}}<h2>Routes</h2>
<div class="card"><table>
<tr><th>Region</th><th class="num">Hops</th><th>Biggest delay step</th></tr>
{{range .R.Regions}}{{if .Route}}
<tr><td><b>{{.Region}}</b></td>
{{if .Route.Error}}<td colspan="2" class="dim">route unknown: {{.Route.Error}}</td>
{{else}}<td class="num{{if not .Route.Reached}} fair{{end}}">{{len .Route.Hops}}{{if not .Route.Reached}}+{{end}}</td>
<td>{{with .Route.Jump}}+{{printf "%.0f" .AddedMs}} ms at hop {{.Hop}}{{end}} <span class="dim">{{where .Route}}</span></td>{{end}}</tr>
{{end}}{{end}}
</table></div>{{end}}

<details><summary>Raw data (JSON)</summary><div class="card"><pre>{{.RawJSON}}</pre></div></details>

<footer>Made with multispeed {{.R.Version}} · servers by Speedtest.net and its volunteer hosts.
Bufferbloat is how much latency rises while the line is busy: A+ under 5 ms, A under 30, B under 60, C under 200, D under 400, F above.</footer>
</main>
</body>
</html>
`))
