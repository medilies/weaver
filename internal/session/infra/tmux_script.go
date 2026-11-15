package infra

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/medilies/weaver/internal/session/domain"
)

// shellLine starts the last line of the startup script, the one that
// starts the shell.
const shellLine = `exec "${SHELL:-/bin/sh}" `

// launch is how the panes of a session start: tmux's default-command and
// default-shell, the tmux the panes call back, and where the logs of the
// panes that clear go.
type launch struct {
	command, shell string
	tmux           string
	logs           string // the project's log directory, "" for none
}

// opening is what a pane runs as it starts: its env, its before commands,
// whether it clears, its note, and last what it types, run when submit.
type opening struct {
	env    []domain.EnvVar
	before []domain.Command
	clears bool
	note   []string
	last   string
	submit bool
}

// openingOf is what the pane runs as it starts.
func openingOf(pane domain.PanePlan) opening {
	o := opening{env: pane.Env(), before: pane.Before(), clears: pane.Clears(), note: pane.Note()}
	if cmd, ok := pane.Startup().Command(); ok {
		o.last, o.submit = cmd.String(), pane.Startup().Submits()
	}
	return o
}

// hookedShell is a shell whose startup weave can add to, by its program's
// name: bash, zsh, fish, or a POSIX sh.
type hookedShell int

const (
	unhooked hookedShell = iota
	bashHook
	zshHook
	fishHook
	shHook
)

// hook is how the shell started by default can be hooked: never when a
// default-command starts something else.
func (l launch) hook() hookedShell {
	if l.command != "" {
		return unhooked
	}
	switch programName(l.shell) {
	case "bash":
		return bashHook
	case "zsh":
		return zshHook
	case "fish":
		return fishHook
	case "sh", "dash", "ash", "ksh", "mksh":
		return shHook
	}
	return unhooked
}

// hooked reports whether o runs from its shell's startup: unseen, out of
// the history, and with its effects kept, when it clears and the shell
// can be hooked.
func (l launch) hooked(o opening) bool {
	return o.clears && l.hook() != unhooked
}

// script is what a pane opening o starts with when a plain shell isn't
// enough, nil otherwise: a short sh script exporting the env and printing
// the note, unless the pane clears, which prints it after its before
// commands, and logging what the pane prints, to log, when it clears; it
// then replaces itself with what tmux would have started: the
// default-command, run by the user's shell as tmux runs it, else the
// user's login shell, which runs o from its startup when hooked. Passed
// as separate arguments, tmux runs it directly rather than through its
// default-shell, so the script is always sh.
func (l launch) script(o opening, log string) []string {
	var lines []string
	for _, v := range o.env {
		lines = append(lines, "export "+v.Name()+"="+shellQuote(v.Value()))
	}
	if !o.clears && len(o.note) > 0 {
		lines = append(lines, printed(o.note))
	}
	if o.clears && log != "" {
		pipe := "cat > " + shellQuote(log)
		lines = append(lines, "mkdir -p "+shellQuote(filepath.Dir(log))+" && "+
			shellQuote(l.tmux)+` pipe-pane -t "$TMUX_PANE" `+shellQuote(formatArgument(pipe)))
	}
	if l.hooked(o) {
		return sh(append(lines, l.hookLines(o, log)...))
	}
	if len(lines) == 0 {
		return nil
	}
	if l.command == "" {
		return sh(append(lines, shellLine+"-l"))
	}
	return sh(append(lines, shellLine+"-c "+shellQuote(l.command)))
}

// sh is a script's lines, as tmux's arguments.
func sh(lines []string) []string {
	return []string{"--", "/bin/sh", "-c", strings.Join(lines, "\n")}
}

// hookLines write the hook running o, in a file or folder of its own,
// $WEAVER_HOOK, which it removes, then start the shell running it once
// the user's own startup ran.
func (l launch) hookLines(o opening, log string) []string {
	switch l.hook() {
	case bashHook:
		// Started with its own rc file, bash is interactive but not a
		// login shell: the hook reads what a login shell would.
		return []string{
			`export WEAVER_HOOK="$(mktemp)"`,
			"printf '%s\\n' " + shellQuote(bashLogin+l.posixHook(o, log, `rm -f -- "$WEAVER_HOOK"; unset WEAVER_HOOK`)) + ` > "$WEAVER_HOOK"`,
			shellLine + `--rcfile "$WEAVER_HOOK" -i`,
		}
	case zshHook:
		// zsh reads its startup files from $ZDOTDIR: the hook's own read
		// the user's, from where they are, the last one running the hook.
		lines := []string{
			`export WEAVER_HOOK="$(mktemp -d)"`,
			`export WEAVER_ZDOTDIR="${ZDOTDIR:-$HOME}"`,
		}
		for _, file := range []string{".zshenv", ".zprofile", ".zshrc"} {
			lines = append(lines, "printf '%s\\n' "+shellQuote(zshReading(file))+` > "$WEAVER_HOOK/`+file+`"`)
		}
		last := zshReading(".zlogin") + "\n" + `ZDOTDIR="$WEAVER_ZDOTDIR"; unset WEAVER_ZDOTDIR` + "\n" +
			l.posixHook(o, log, `rm -rf -- "$WEAVER_HOOK"; unset WEAVER_HOOK`)
		lines = append(lines, "printf '%s\\n' "+shellQuote(last)+` > "$WEAVER_HOOK/.zlogin"`)
		return append(lines, `export ZDOTDIR="$WEAVER_HOOK"`, shellLine+"-l")
	case fishHook:
		return []string{
			`export WEAVER_HOOK="$(mktemp)"`,
			"printf '%s\\n' " + shellQuote(l.fishHook(o, log)) + ` > "$WEAVER_HOOK"`,
			shellLine + `-l -C "source '$WEAVER_HOOK'"`,
		}
	}
	// A POSIX sh, interactive, reads $ENV last: the hook reads the user's.
	return []string{
		`export WEAVER_HOOK="$(mktemp)"`,
		"printf '%s\\n' " + shellQuote(shEnv+l.posixHook(o, log, `rm -f -- "$WEAVER_HOOK"; unset WEAVER_HOOK`)) + ` > "$WEAVER_HOOK"`,
		`export WEAVER_ENV="${ENV-}"`,
		`export ENV="$WEAVER_HOOK"`,
		shellLine + "-l",
	}
}

// bashLogin reads the startup files a login bash reads.
const bashLogin = `if [ -r /etc/profile ]; then . /etc/profile; fi
for __weaver_f in ~/.bash_profile ~/.bash_login ~/.profile; do
  if [ -r "$__weaver_f" ]; then . "$__weaver_f"; break; fi
done
unset __weaver_f
`

// shEnv reads the file $ENV named before the hook took its place.
const shEnv = `ENV="$WEAVER_ENV"; unset WEAVER_ENV
if [ -n "$ENV" ]; then . "$ENV"; else unset ENV; fi
`

// zshReading reads the user's own startup file, from their $ZDOTDIR,
// which their .zshenv may change, then gives zsh back the hook's.
func zshReading(file string) string {
	return `ZDOTDIR="$WEAVER_ZDOTDIR"
if [[ -r "$ZDOTDIR/` + file + `" ]]; then . "$ZDOTDIR/` + file + `"; fi
WEAVER_ZDOTDIR="$ZDOTDIR"; ZDOTDIR="$WEAVER_HOOK"`
}

// posixHook runs o in bash, zsh or a POSIX sh: its before commands, each
// whole, noting a failure, or ctrl+c, which would otherwise end the
// startup there; then the log closed; then what o types last
// typed, as if by the user, so it stays in the history, before the screen
// is cleared, which hides the terminal echoing it ahead of the prompt;
// then the note printed, the failure told, and cleanup.
func (l launch) posixHook(o opening, log, cleanup string) string {
	lines := []string{"__weaver_failed=", "trap '__weaver_failed=1' INT"}
	for _, cmd := range o.before {
		lines = append(lines, "{ "+cmd.String()+"\n} || __weaver_failed=1")
	}
	lines = append(lines, "trap - INT")
	lines = append(lines, l.closing(o, log, `"$TMUX_PANE"`)...)
	lines = append(lines,
		`[ -z "$__weaver_failed" ] || `+printed([]string{failedLine(log)}),
		"unset __weaver_failed")
	return strings.Join(append(lines, cleanup), "\n")
}

// fishHook is posixHook for fish.
func (l launch) fishHook(o opening, log string) string {
	lines := []string{"set -l __weaver_failed"}
	for _, cmd := range o.before {
		lines = append(lines, "begin\n"+cmd.String()+"\nend; or set __weaver_failed 1")
	}
	lines = append(lines, l.closing(o, log, "$TMUX_PANE")...)
	lines = append(lines, `test -z "$__weaver_failed"; or `+printed([]string{failedLine(log)}))
	return strings.Join(append(lines, `rm -f -- "$WEAVER_HOOK"; set -e WEAVER_HOOK`), "\n")
}

// closing closes the log of the pane, types what o types last, clears
// the screen and prints the note.
func (l launch) closing(o opening, log, pane string) []string {
	var lines []string
	if log != "" {
		lines = append(lines, shellQuote(l.tmux)+" pipe-pane -t "+pane)
	}
	lines = append(lines, l.typing(o, pane)...)
	lines = append(lines, "clear")
	if len(o.note) > 0 {
		lines = append(lines, printed(o.note))
	}
	return lines
}

// typing types what o types last into the pane, pressing Enter when it
// submits.
func (l launch) typing(o opening, pane string) []string {
	if o.last == "" {
		return nil
	}
	lines := []string{shellQuote(l.tmux) + " send-keys -t " + pane + " -l -- " + shellQuote(argument(o.last))}
	if o.submit {
		lines = append(lines, shellQuote(l.tmux)+" send-keys -t "+pane+" Enter")
	}
	return lines
}

// failedLine tells that a before command failed, or was interrupted, and
// where what it printed is.
func failedLine(log string) string {
	if log == "" {
		return "✗ a before command failed or was interrupted"
	}
	return "✗ a before command failed or was interrupted: what it printed is in " + log
}

// typedLine is a line weave types into a pane, run when submit.
type typedLine struct {
	text   string
	submit bool
}

// typed is what weave types into the pane id opening o, once started,
// unless its shell's startup runs it: its before commands; when it
// clears, the log closed, clear, and the note printed, a space first
// leaving them out of the history of shells told to ignore such lines;
// then what it types last.
func (l launch) typed(o opening, id, log string) []typedLine {
	if l.hooked(o) {
		return nil
	}
	var lines []typedLine
	for _, cmd := range o.before {
		lines = append(lines, typedLine{text: cmd.String(), submit: true})
	}
	if o.clears {
		cleared := opening{note: o.note} // what o types last is typed after
		lines = append(lines, typedLine{text: " " + strings.Join(l.closing(cleared, log, id), "; "), submit: true})
	}
	if o.last != "" {
		lines = append(lines, typedLine{text: o.last, submit: o.submit})
	}
	return lines
}

// logOf is the log named name, as "window.pane", under the project's log
// directory: "" without one.
func (l launch) logOf(name string) string {
	if l.logs == "" {
		return ""
	}
	return filepath.Join(l.logs, strings.ReplaceAll(name, "/", "_")+".log")
}

// paneLog is the log of the pane at index of window, named after the
// pane, or numbered from 1 when it has no name.
func (l launch) paneLog(window domain.WindowPlan, index int) string {
	pane := window.Panes()[index].Name().String()
	if pane == "" {
		pane = strconv.Itoa(index + 1)
	}
	return l.logOf(window.Name().String() + "." + pane)
}

// respawn is what a respawned pane opening o starts with: its script, or
// a plain shell, which the session's panes start as when none is given,
// since respawn-pane would run the pane's first command again.
func (l launch) respawn(o opening, log string) []string {
	if script := l.script(o, log); script != nil {
		return script
	}
	if l.command != "" {
		return sh([]string{shellLine + "-c " + shellQuote(l.command)})
	}
	return sh([]string{shellLine + "-l"})
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
// script of launch.script's, so the pane runs a shell like any other.
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
