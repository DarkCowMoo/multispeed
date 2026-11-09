package main

import (
	"fmt"
	"io"
	"strings"
)

// Map size and the part of the world it shows (an equirectangular projection).
const (
	mapWidth  = 78
	mapHeight = 22
	mapNorth  = 78.0
	mapSouth  = -56.0
	mapWest   = -170.0
	mapEast   = 190.0
)

// continents are rough outlines as (lon, lat) points. At about 5 degrees per
// character, rough is all a terminal map needs.
var continents = [][][2]float64{
	// North and Central America
	{{-168, 65}, {-165, 70}, {-155, 71}, {-140, 70}, {-128, 70}, {-115, 68}, {-95, 72}, {-80, 73},
		{-65, 62}, {-55, 52}, {-66, 45}, {-70, 42}, {-76, 35}, {-81, 31}, {-80, 25}, {-83, 29},
		{-90, 30}, {-97, 26}, {-97, 21}, {-90, 21}, {-87, 16}, {-83, 9}, {-77, 8}, {-80, 7},
		{-85, 11}, {-92, 15}, {-105, 20}, {-110, 24}, {-115, 30}, {-118, 34}, {-124, 40},
		{-124, 48}, {-130, 55}, {-140, 60}, {-152, 59}, {-158, 57}, {-165, 60}},
	// Greenland
	{{-55, 60}, {-45, 60}, {-20, 70}, {-20, 82}, {-60, 82}, {-72, 78}, {-58, 70}},
	// South America
	{{-80, 9}, {-72, 12}, {-62, 11}, {-50, 0}, {-35, -6}, {-39, -15}, {-48, -26}, {-58, -38},
		{-65, -42}, {-68, -52}, {-75, -50}, {-73, -40}, {-71, -30}, {-71, -18}, {-76, -14},
		{-81, -5}, {-80, 1}, {-77, 7}},
	// Europe and Asia
	{{-9, 37}, {-9, 43}, {-2, 44}, {-5, 48}, {2, 51}, {8, 54}, {10, 57}, {5, 58}, {5, 62},
		{15, 69}, {25, 71}, {40, 68}, {45, 68}, {60, 70}, {70, 73}, {80, 73}, {100, 78},
		{115, 74}, {140, 73}, {160, 70}, {180, 68}, {190, 66}, {180, 64}, {170, 60}, {163, 56},
		{155, 60}, {143, 59}, {135, 54}, {141, 47}, {130, 42}, {127, 35}, {122, 40}, {118, 38},
		{122, 30}, {120, 23}, {110, 20}, {108, 12}, {105, 9}, {100, 13}, {100, 7}, {103, 1},
		{98, 9}, {94, 16}, {92, 22}, {88, 22}, {80, 15}, {77, 8}, {73, 17}, {68, 24}, {58, 25},
		{56, 22}, {52, 16}, {43, 12}, {39, 21}, {35, 28}, {34, 31}, {35, 36}, {28, 36},
		{26, 40}, {23, 37}, {19, 40}, {16, 38}, {15, 40}, {12, 44}, {8, 44}, {3, 43},
		{-1, 37}, {-6, 36}},
	// Africa
	{{-17, 21}, {-17, 15}, {-12, 8}, {-5, 5}, {5, 4}, {9, 4}, {10, -2}, {12, -6}, {13, -12},
		{12, -18}, {15, -27}, {18, -34}, {26, -34}, {33, -28}, {35, -20}, {40, -15}, {40, -5},
		{43, 0}, {51, 11}, {43, 12}, {37, 20}, {33, 31}, {25, 32}, {20, 31}, {10, 37}, {-1, 36},
		{-6, 35}, {-10, 30}, {-15, 25}},
	// Australia
	{{113, -22}, {115, -34}, {124, -34}, {132, -32}, {138, -35}, {146, -39}, {150, -38},
		{151.8, -33.5}, {153.6, -28}, {146, -19}, {142, -11}, {136, -12}, {130, -12}, {126, -14},
		{122, -18}},
	// Great Britain
	{{-5, 50}, {1, 51}, {2, 53}, {-2, 56}, {-3, 58}, {-6, 58}, {-5, 55}, {-3, 54}, {-5, 52}},
	// Iceland
	{{-24, 64}, {-14, 64}, {-15, 66}, {-22, 66}},
	// Japan
	{{130, 31}, {135, 34}, {140, 35}, {142, 40}, {141, 45}, {144, 44}, {140, 38}, {136, 36}, {132, 34}},
	// Sumatra, Borneo, New Guinea, Philippines
	{{95, 5}, {105, -6}, {106, -3}, {100, 2}},
	{{109, 1}, {117, 7}, {119, 1}, {116, -4}, {110, -3}},
	{{131, -1}, {141, -3}, {150, -10}, {141, -9}, {137, -5}},
	{{120, 18}, {122, 18}, {126, 7}, {122, 7}},
	// New Zealand, Madagascar
	{{166, -46}, {174, -41}, {178, -38}, {173, -35}, {172, -41}},
	{{44, -25}, {47, -25}, {50, -15}, {49, -12}, {44, -17}},
}

// isLand reports whether a point falls inside any continent outline.
func isLand(lon, lat float64) bool {
	for _, poly := range continents {
		if inPolygon(lon, lat, poly) {
			return true
		}
	}
	return false
}

// inPolygon is the even-odd ray casting test.
func inPolygon(x, y float64, poly [][2]float64) bool {
	in := false
	for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
		xi, yi := poly[i][0], poly[i][1]
		xj, yj := poly[j][0], poly[j][1]
		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			in = !in
		}
	}
	return in
}

// mapCell converts a location to a map cell.
func mapCell(lat, lon float64) (col, row int) {
	if lon < mapWest {
		lon += 360
	}
	col = int((lon - mapWest) / (mapEast - mapWest) * mapWidth)
	row = int((mapNorth - lat) / (mapNorth - mapSouth) * mapHeight)
	return min(max(col, 0), mapWidth-1), min(max(row, 0), mapHeight-1)
}

// mapMarker is one region drawn on the map.
type mapMarker struct {
	label    rune
	col, row int
	s        *RegionSummary
}

// placeMarkers gives each region with a location a letter and a free cell
// near its true position, so nearby cities (London, Amsterdam, Frankfurt)
// don't hide each other.
func placeMarkers(sums []RegionSummary) []mapMarker {
	taken := map[[2]int]bool{}
	var out []mapMarker
	label := 'A'
	for i := range sums {
		s := &sums[i]
		if s.Lat == 0 && s.Lon == 0 {
			continue // unknown location
		}
		col, row := mapCell(s.Lat, s.Lon)
		col, row = freeCell(taken, col, row)
		taken[[2]int{col, row}] = true
		out = append(out, mapMarker{label: label, col: col, row: row, s: s})
		label++
	}
	return out
}

// freeCell finds the nearest free cell, preferring the same row.
func freeCell(taken map[[2]int]bool, col, row int) (int, int) {
	for dist := 0; dist < mapWidth; dist++ {
		for _, d := range [][2]int{{dist, 0}, {-dist, 0}, {dist, 1}, {-dist, 1}, {dist, -1}, {-dist, -1}} {
			c, r := col+d[0], row+d[1]
			if c >= 0 && c < mapWidth && r >= 0 && r < mapHeight && !taken[[2]int{c, r}] {
				return c, r
			}
		}
	}
	return col, row
}

// printMap draws the world with a colored letter for each region and a
// legend below. Regions are colored by download speed against the best one.
func printMap(w io.Writer, sums []RegionSummary, p palette) error {
	markers := placeMarkers(sums)
	if len(markers) < 2 {
		return nil
	}
	best := 0.0
	for i := range sums {
		best = max(best, sums[i].BestDownMbps)
	}
	at := map[[2]int]mapMarker{}
	for _, m := range markers {
		at[[2]int{m.col, m.row}] = m
	}
	style := func(s *RegionSummary, text string) string {
		if s.OK == 0 {
			return p.bad(text)
		}
		return p.speed(s.BestDownMbps, best, text)
	}

	var b strings.Builder
	for row := 0; row < mapHeight; row++ {
		var line strings.Builder
		for col := 0; col < mapWidth; col++ {
			if m, ok := at[[2]int{col, row}]; ok {
				line.WriteString(p.bold(style(m.s, string(m.label))))
				continue
			}
			lon := mapWest + (float64(col)+0.5)/mapWidth*(mapEast-mapWest)
			lat := mapNorth - (float64(row)+0.5)/mapHeight*(mapNorth-mapSouth)
			if lon > 180 {
				lon -= 360
			}
			if isLand(lon, lat) {
				line.WriteString(p.dim("·"))
			} else {
				line.WriteByte(' ')
			}
		}
		b.WriteString(strings.TrimRight("  "+line.String(), " ") + "\n")
	}

	// Legend, wrapped to the map width.
	b.WriteString("\n")
	lineLen := 0
	b.WriteString("  ")
	for _, m := range markers {
		speed := "no result"
		if m.s.OK > 0 {
			speed = fmt.Sprintf("%.0f Mbps", m.s.BestDownMbps)
		}
		entry := fmt.Sprintf("%c %s %s", m.label, shortRegion(m.s.Region), speed)
		if lineLen > 0 && lineLen+width(entry)+3 > mapWidth {
			b.WriteString("\n  ")
			lineLen = 0
		} else if lineLen > 0 {
			b.WriteString("   ")
			lineLen += 3
		}
		b.WriteString(p.bold(style(m.s, string(m.label))) + " " +
			shortRegion(m.s.Region) + " " + p.dim(speed))
		lineLen += width(entry)
	}
	b.WriteString("\n")
	_, err := io.WriteString(w, b.String())
	return err
}
