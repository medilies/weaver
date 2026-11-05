package infra

import (
	"strings"

	"github.com/medilies/weaver/internal/session/domain"
)

// shellCommand is what a pane starts with when a plain shell isn't
// enough: a short script exporting env vars and printing the note, which
// then replaces itself with the user's login shell, as tmux would have
// started. Passed as separate arguments,
// tmux runs it directly rather than through its default-shell, so the
// script is always sh. It is nil for a plain shell.
func shellCommand(pane domain.PanePlan) []string {
	var lines []string
	for _, v := range pane.Env() {
		lines = append(lines, "export "+v.Name()+"="+shellQuote(v.Value()))
	}
	if note := pane.Note(); len(note) > 0 {
		quoted := make([]string, len(note))
		for i, line := range note {
			quoted[i] = shellQuote(line)
		}
		lines = append(lines, `printf '%s\n' `+strings.Join(quoted, " "))
	}
	if len(lines) == 0 {
		return nil
	}
	lines = append(lines, `exec "${SHELL:-/bin/sh}" -l`)
	return []string{"--", "/bin/sh", "-c", strings.Join(lines, "\n")}
}

// shellQuote quotes s for sh, so it is taken literally.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
