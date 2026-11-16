package infra

import (
	"strings"

	"github.com/medilies/weaver/internal/session/domain"
)

// shellLine starts the last line of the startup script, the one that
// starts the shell.
const shellLine = `exec "${SHELL:-/bin/sh}" `

// shellCommand is what a pane starts with when a plain shell isn't
// enough: a short script exporting env vars and printing the note, unless
// the pane clears, which prints it after its before commands; it
// then replaces itself with what tmux would have started: defaultCommand,
// tmux's default-command, run by the user's shell as tmux runs it, else
// the user's login shell. Passed as separate arguments, tmux runs it
// directly rather than through its default-shell, so the script is always
// sh. It is nil for a plain shell.
func shellCommand(pane domain.PanePlan, defaultCommand string) []string {
	var note []string
	if !pane.Clears() {
		note = pane.Note()
	}
	return startScript(pane.Env(), note, defaultCommand)
}

// startScript is shellCommand's script exporting env and printing note,
// or nil when there is neither.
func startScript(env []domain.EnvVar, note []string, defaultCommand string) []string {
	var lines []string
	for _, v := range env {
		lines = append(lines, "export "+v.Name()+"="+shellQuote(v.Value()))
	}
	if len(note) > 0 {
		lines = append(lines, printed(note))
	}
	if len(lines) == 0 {
		return nil
	}
	// tmux sets SHELL to its default-shell in every pane it starts.
	if defaultCommand == "" {
		lines = append(lines, shellLine+"-l")
	} else {
		lines = append(lines, shellLine+"-c "+shellQuote(defaultCommand))
	}
	return []string{"--", "/bin/sh", "-c", strings.Join(lines, "\n")}
}

// clearLine is what a pane that clears types after its before commands:
// clear, then its note printed. A space first leaves it out of the
// history of shells told to ignore such lines.
func clearLine(pane domain.PanePlan) string {
	line := " clear"
	if note := pane.Note(); len(note) > 0 {
		line += "; " + printed(note)
	}
	return line
}

// printed is a command printing lines, one per line.
func printed(lines []string) string {
	quoted := make([]string, len(lines))
	for i, line := range lines {
		quoted[i] = shellQuote(line)
	}
	return `printf '%s\n' ` + strings.Join(quoted, " ")
}

// wrapped reports whether args, a pane's start command, are a startup
// script of shellCommand's, so the pane runs a shell like any other.
func wrapped(args []string) bool {
	if len(args) != 3 || args[0] != "/bin/sh" || args[1] != "-c" {
		return false
	}
	script := args[2]
	return strings.HasPrefix(script[strings.LastIndexByte(script, '\n')+1:], shellLine)
}

// shellQuote quotes s for sh, so it is taken literally.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellWords is args as a command line sh splits back into them, each
// quoted only when it has to be.
func shellWords(args []string) string {
	words := make([]string, len(args))
	for i, arg := range args {
		words[i] = arg
		// A first word with "=" would be read as setting a variable, and
		// zsh expands a word starting with "=" into a program's path.
		if arg == "" || strings.ContainsFunc(arg, needsQuotes) || strings.HasPrefix(arg, "=") ||
			i == 0 && strings.Contains(arg, "=") {
			words[i] = shellQuote(arg)
		}
	}
	return strings.Join(words, " ")
}

// needsQuotes reports whether sh could read r as more than itself.
func needsQuotes(r rune) bool {
	switch {
	case 'a' <= r && r <= 'z', 'A' <= r && r <= 'Z', '0' <= r && r <= '9':
		return false
	}
	return !strings.ContainsRune("-_./:,+=@%", r)
}
