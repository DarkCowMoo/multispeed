// Package regions defines the places multispeed can test: preset internet hubs,
// "local" (your nearest servers), and custom Name=lat,lon locations.
package regions

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Region is a place we want to measure our connection to.
// Servers are discovered automatically near (Lat, Lon).
type Region struct {
	Name string
	Lat  float64
	Lon  float64
	// Local means "near me": use Speedtest.net's own nearest-server list.
	Local bool
}

// Presets are well-known internet hubs where Speedtest.net has many servers.
var Presets = map[string]Region{
	"amsterdam":    {Name: "Amsterdam", Lat: 52.3676, Lon: 4.9041},
	"athens":       {Name: "Athens", Lat: 37.9838, Lon: 23.7275},
	"bangkok":      {Name: "Bangkok", Lat: 13.7563, Lon: 100.5018},
	"chicago":      {Name: "Chicago", Lat: 41.8781, Lon: -87.6298},
	"dallas":       {Name: "Dallas", Lat: 32.7767, Lon: -96.7970},
	"dubai":        {Name: "Dubai", Lat: 25.2048, Lon: 55.2708},
	"frankfurt":    {Name: "Frankfurt", Lat: 50.1109, Lon: 8.6821},
	"hongkong":     {Name: "Hong Kong", Lat: 22.3193, Lon: 114.1694},
	"istanbul":     {Name: "Istanbul", Lat: 41.0082, Lon: 28.9784},
	"johannesburg": {Name: "Johannesburg", Lat: -26.2041, Lon: 28.0473},
	"london":       {Name: "London", Lat: 51.5072, Lon: -0.1276},
	"losangeles":   {Name: "Los Angeles", Lat: 34.0522, Lon: -118.2437},
	"madrid":       {Name: "Madrid", Lat: 40.4168, Lon: -3.7038},
	"marseille":    {Name: "Marseille", Lat: 43.2965, Lon: 5.3698},
	"miami":        {Name: "Miami", Lat: 25.7617, Lon: -80.1918},
	"milan":        {Name: "Milan", Lat: 45.4642, Lon: 9.1900},
	"mumbai":       {Name: "Mumbai", Lat: 19.0760, Lon: 72.8777},
	"newyork":      {Name: "New York", Lat: 40.7128, Lon: -74.0060},
	"paris":        {Name: "Paris", Lat: 48.8566, Lon: 2.3522},
	"saopaulo":     {Name: "São Paulo", Lat: -23.5505, Lon: -46.6333},
	"seattle":      {Name: "Seattle", Lat: 47.6062, Lon: -122.3321},
	"singapore":    {Name: "Singapore", Lat: 1.3521, Lon: 103.8198},
	"stockholm":    {Name: "Stockholm", Lat: 59.3293, Lon: 18.0686},
	"sydney":       {Name: "Sydney", Lat: -33.8688, Lon: 151.2093},
	"telaviv":      {Name: "Tel Aviv", Lat: 32.0853, Lon: 34.7818},
	"tokyo":        {Name: "Tokyo", Lat: 35.6762, Lon: 139.6503},
	"toronto":      {Name: "Toronto", Lat: 43.6532, Lon: -79.3832},
	"warsaw":       {Name: "Warsaw", Lat: 52.2297, Lon: 21.0122},
}

// DefaultRegions are tested when no regions are given: your local connection plus 9 major hubs around the world.
var DefaultRegions = []string{
	"local", "frankfurt", "london", "amsterdam", "newyork",
	"losangeles", "singapore", "tokyo", "mumbai", "sydney",
}

// ParseRegions turns "local,frankfurt,Lisbon=38.72,-9.14" into regions.
// Each item is "local", a preset name, or "Name=lat,lon".
func ParseRegions(spec string) ([]Region, error) {
	var out []Region
	items := splitRegionSpec(spec)
	for _, item := range items {
		r, err := parseRegion(item)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no regions given")
	}
	return out, nil
}

// splitRegionSpec splits on commas, but keeps "Name=lat,lon" together.
func splitRegionSpec(spec string) []string {
	parts := strings.Split(spec, ",")
	var items []string
	for i := 0; i < len(parts); i++ {
		p := strings.TrimSpace(parts[i])
		if p == "" {
			continue
		}
		if strings.Contains(p, "=") && i+1 < len(parts) {
			p = p + "," + strings.TrimSpace(parts[i+1])
			i++
		}
		items = append(items, p)
	}
	return items
}

func parseRegion(item string) (Region, error) {
	if name, coords, ok := strings.Cut(item, "="); ok {
		lat, lon, err := parseLatLon(coords)
		if err != nil {
			return Region{}, fmt.Errorf("region %q: %w", item, err)
		}
		return Region{Name: strings.TrimSpace(name), Lat: lat, Lon: lon}, nil
	}
	key := Normalize(item)
	if key == "local" || key == "nearest" {
		return Region{Name: "Local (nearest)", Local: true}, nil
	}
	if r, ok := Presets[key]; ok {
		return r, nil
	}
	return Region{}, fmt.Errorf("unknown region %q (run with -list-regions, or use Name=lat,lon)", item)
}

func parseLatLon(s string) (float64, float64, error) {
	a, b, ok := strings.Cut(s, ",")
	if !ok {
		return 0, 0, fmt.Errorf("expected lat,lon")
	}
	lat, err := strconv.ParseFloat(strings.TrimSpace(a), 64)
	if err != nil || lat < -90 || lat > 90 {
		return 0, 0, fmt.Errorf("bad latitude %q", a)
	}
	lon, err := strconv.ParseFloat(strings.TrimSpace(b), 64)
	if err != nil || lon < -180 || lon > 180 {
		return 0, 0, fmt.Errorf("bad longitude %q", b)
	}
	return lat, lon, nil
}

// Normalize turns a region name into its preset key: "New York" -> "newyork".
func Normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.NewReplacer(" ", "", "-", "", "_", "", ".", "").Replace(s)
}

// PresetNames lists the preset keys in alphabetical order.
func PresetNames() []string {
	names := make([]string, 0, len(Presets))
	for k := range Presets {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
