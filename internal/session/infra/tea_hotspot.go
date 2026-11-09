package infra

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// hotspot is the cells of a line a click acts on: a key told, doing
// what the key does, or, without a key, the item of a list it selects,
// a window in the window list or a pane in the pane list.
type hotspot struct {
	from, to int
	key      string
	item     int
}

// hotspotAt is the hotspot of spots covering column x, if any.
func hotspotAt(spots []hotspot, x int) (hotspot, bool) {
	for _, spot := range spots {
		if x >= spot.from && x < spot.to {
			return spot, true
		}
	}
	return hotspot{}, false
}

// cut is line cut to width cells, "…" in place of the rest, and the
// spots on it clipped to what is still shown: the "…" acts on nothing.
func cut(line string, spots []hotspot, width int) (string, []hotspot) {
	limit := width
	if ansi.StringWidth(line) > width {
		limit = max(width-1, 0)
	}
	var kept []hotspot
	for _, spot := range spots {
		if spot.from < limit {
			spot.to = min(spot.to, limit)
			kept = append(kept, spot)
		}
	}
	return ansi.Truncate(line, width, "…"), kept
}

// listed is labels side by side, a space apart, with the cells each one
// covers, to click it. When they don't fit in width, the first ones make
// way for the selected one, a "…" in their place, and the line is cut
// to width.
func listed(labels []string, selected, width int) (string, []hotspot) {
	first := 0
	line := strings.Join(labels, " ")
	for first < selected && ansi.StringWidth(line) > width {
		first++
		line = "…" + strings.Join(labels[first:], " ")
	}
	var spots []hotspot
	from := 0
	if first > 0 {
		from = 1 // the "…"
	}
	for i := first; i < len(labels); i++ {
		to := from + ansi.StringWidth(labels[i])
		spots = append(spots, hotspot{from: from, to: to, item: i})
		from = to + 1
	}
	return cut(line, spots, width)
}
