package infra

import "strings"

// diffContext is the unchanged lines shown around each change.
const diffContext = 2

// configDiff is what changes from before to after, line by line: "- " for
// a line removed, "+ " for one added, "  " for one kept, which shows only
// within diffContext lines of a change, and "⋯" where kept lines are left
// out. It changes the fewest lines, and is empty when nothing changes.
func configDiff(before, after string) []string {
	lines := diffed(splitLines(before), splitLines(after))
	near := make([]bool, len(lines))
	for i, line := range lines {
		if line[0] != ' ' {
			for j := max(i-diffContext, 0); j <= min(i+diffContext, len(lines)-1); j++ {
				near[j] = true
			}
		}
	}
	var shown []string
	skipped := false
	for i, line := range lines {
		if !near[i] {
			skipped = true
			continue
		}
		if skipped {
			shown = append(shown, "⋯")
			skipped = false
		}
		shown = append(shown, line)
	}
	if skipped && len(shown) > 0 {
		shown = append(shown, "⋯")
	}
	return shown
}

// diffed is a's and b's lines in order, each prefixed "- " when only a has
// it, "+ " when only b has it and "  " when both do, keeping their longest
// common subsequence.
func diffed(a, b []string) []string {
	// common[i][j] is the length of the longest common subsequence of
	// a[i:] and b[j:].
	common := make([][]int, len(a)+1)
	for i := range common {
		common[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				common[i][j] = common[i+1][j+1] + 1
			} else {
				common[i][j] = max(common[i+1][j], common[i][j+1])
			}
		}
	}
	var out []string
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			out = append(out, "  "+a[i])
			i, j = i+1, j+1
		case j == len(b) || i < len(a) && common[i+1][j] >= common[i][j+1]:
			out = append(out, "- "+a[i])
			i++
		default:
			out = append(out, "+ "+b[j])
			j++
		}
	}
	return out
}

// splitLines is text's lines, without their line ends.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}
