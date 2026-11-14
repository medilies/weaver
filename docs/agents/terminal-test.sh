#!/usr/bin/env bash
# Runs the scenarios of docs/agents/terminal-testing.md against a
# throwaway tmux server: one line per scenario, and the screen of a
# failing one.
#
#   terminal-test.sh [-b DIR] [-k] [N...]
#
#   N...     the scenarios to run, all by default; each sets up what it
#            needs, so any one runs alone
#   -b DIR   test DIR/weave, such as a release's, instead of building
#            ./cmd/weave
#   -k       keep /tmp/wvt afterwards, to look around or watch
#
# Watch a run, read-only, from any terminal of 160x45 or more:
#   tmux -S /tmp/wvt/screen.sock attach -r
# Sourced, after a run with -k, it defines its functions and runs
# nothing: unset TMUX; . docs/agents/terminal-test.sh; fresh; browse shop
#
# Never touches the user's tmux: every tmux command names its socket.
set -u
export LC_ALL=C.UTF-8 # characters, not bytes, in ${#…} and grep

W=/tmp/wvt
SOCK=$W/tmux/tmux-$(id -u)/default
SCREEN=$W/screen.sock
COLS=160 ROWS=45

# Every tmux command runs in the test's environment, not the caller's:
# tmux gives the panes a command creates the environment of the client
# sending it, PATH included.
TEST_ENV=(HOME="$W/home" LANG=C.UTF-8 LC_ALL=C.UTF-8 TERM=tmux-256color
	TMUX_TMPDIR="$W/tmux" XDG_CONFIG_HOME="$W/cfg"
	PATH="$W/bin:$(dirname "$(command -v tmux)"):/usr/bin:/bin")
T() { (cd "$W" && env -i "${TEST_ENV[@]}" tmux -S "$SOCK" "$@"); }
S() { (cd "$W" && env -i "${TEST_ENV[@]}" tmux -S "$SCREEN" "$@"); }

# AT is the pane a scenario types into and reads: term's, unless it
# says otherwise.
AT==term:

# --- Waiting: poll for what must show, never sleep a fixed time ---------------

# until_ <seconds> <command...>: the command succeeded within the
# seconds, tried every tenth of one.
until_() {
	local tries=$(($1 * 10)) i
	shift
	for ((i = 0; i < tries; i++)); do
		"$@" && return 0
		sleep 0.1
	done
	return 1
}

screen() { T capture-pane -p -t "${1:-$AT}"; }
shows() { screen | grep -qF -- "$1"; }
lacks() { ! shows "$1"; }
has_line() { screen "$1" | grep -qx -- "$2"; }
changed() { [ "$(screen)" != "$1" ]; }
on() { [ "$(T list-clients -F '#{client_session}' 2>/dev/null)" = "$1" ]; }
runs() { [ "$(T display -p -t "$1" '#{pane_current_command}' 2>/dev/null)" = "$2" ]; }
ended() { ! T has-session -t "=$1" 2>/dev/null; }

# pane_of <session:window> <name>: the ID of the pane with that name.
pane_of() { T list-panes -t "=$1" -F '#{pane_id} #{@label}' | awk -v n="$2" '$2 == n {print $1}'; }

# --- Checking: the first failed check ends the scenario -----------------------

FAILED=
fail() {
	FAILED=$1
	return 1
}

wait_for() { until_ "${2:-5}" shows "$1" || fail "'$1' never showed"; }
wait_gone() { until_ "${2:-5}" lacks "$1" || fail "'$1' stayed"; }
wait_on() { until_ "${2:-5}" on "$1" || fail "the client is on '$(T list-clients -F '#{client_session}')', not '$1'"; }
wait_runs() { until_ "${3:-5}" runs "$1" "$2" || fail "$1 runs '$(T display -p -t "$1" '#{pane_current_command}')', not '$2'"; }
expect() { [ "$2" = "$3" ] || fail "$1 is '$2', not '$3'"; }

# keys <key...>: keys for AT. Bubble Tea reads keys in order, so keys
# sent together are safe; wait only for a program to start.
keys() { T send-keys -t "$AT" "$@"; }

# step <key...>: the keys, then a wait until the screen changes.
step() {
	local before
	before=$(screen)
	keys "$@"
	until_ 2 changed "$before" || fail "'$*' changed nothing"
}

# cursor_to <pattern>: Down until the config form's cursor, ›, is on the
# row matching the pattern.
cursor_to() {
	local i
	for ((i = 0; i < 60; i++)); do
		screen | grep -qE -- "› +$1" && return 0
		step Down || return
	done
	fail "the cursor never reached '$1'"
}

# run <command>: the command typed into AT's shell.
run() {
	wait_runs "$AT" bash || return
	keys "clear; $1" Enter
}

# browse [project]: the browser in AT, on the project, which the filter
# selects.
browse() {
	run weave || return
	wait_for 'q quit' || return
	[ -z "${1:-}" ] && return
	keys / "$1" Enter
	wait_for "$1.yml"
}

# form <command>: the config form in AT.
form() {
	run "$1" || return
	wait_for '↑↓ move'
}

# split_at: the column the drawing's second pane starts at, its prompt
# on the same line as the first's.
split_at() {
	local line
	line=$(screen | grep -m1 -E '\$ █.*\$ █')
	line=${line%\$ █*}
	echo ${#line}
}

# --- Fixtures ---------------------------------------------------------------

write_projects() {
	local p=$W/cfg/weaver/projects
	rm -f "$p"/*.yml "$W/cfg/weaver/settings.yml"
	cat >"$p/shop.yml" <<-'EOF'
		root: /tmp/wvt/shop
		env: { APP_ENV: local, LOG: info }
		setup: [echo setting up shop]
		teardown: [echo tearing down, "false"]
		windows:
		  - name: shell
		  - name: api
		    focus: true
		    env: { APP_ENV: staging }
		    panes:
		      - name: logs
		        command: sleep 3
		        tasks: [{name: wait, command: sleep 300}, {command: echo hi}]
		      - name: back
		        focus: true
		        dir: back
		        env: { APP_ENV: dev }
		        split: { direction: right, size: 60% }
	EOF
	cat >"$p/blog.yml" <<-'EOF'
		root: /tmp/wvt/blog
		windows:
		  - name: write
		    grid:
		      columns: [60%, 40%]
		      rows: [70%, 30%]
		      areas:
		        - [editor, preview]
		        - [editor, shell]
		    panes:
		      - name: editor
		      - name: preview
		        focus: true
		      - name: shell
	EOF
	cat >"$p/notes.yml" <<-'EOF'
		root: /tmp/wvt/notes
		windows:
		  - name: code
		    grid:
		      columns: [30, 1fr]
		      areas:
		        - [files, edit]
		    panes:
		      - name: files
		        tasks: [{command: ls}]
		      - name: edit
		        tasks: [{command: echo saved}]
		  - name: shell
	EOF
}

setup() {
	mkdir -p "$(dirname "$SOCK")" "$W/cfg/weaver/projects" "$W/home" "$W/bin" \
		"$W/shop/back" "$W/blog" "$W/notes"
	chmod 700 "$(dirname "$SOCK")"
	if [ -n "$1" ]; then
		cp "$1/weave" "$W/bin/weave" || exit 2
	else
		go build -o "$W/bin/weave" ./cmd/weave || exit 2
	fi
	write_projects
	# The same shells on every machine: a home of their own, whose
	# .bashrc runs after the system's, a plain prompt, no login banner.
	printf '%s\n' "PS1='\\$ '" >"$W/home/.bashrc"
	touch "$W/home/.hushlogin"
	printf '%s\n' 'set -g default-terminal tmux-256color' >"$W/tmux.conf"
	T -f "$W/tmux.conf" new-session -d -s term -x $COLS -y $ROWS bash --norc --noprofile
	# The terminal attaching and switching: a pane of a second server, of
	# a fixed size whoever watches it, attaching again when a kill ends
	# its client.
	cat >"$W/attach.sh" <<-EOF
		while tmux -S $SOCK has-session -t term 2>/dev/null; do
		  tmux -S $SOCK attach -t term
		  sleep 0.2
		done
	EOF
	S -f /dev/null new-session -d -s screen -x $COLS -y $ROWS sh "$W/attach.sh"
	S set -g status off
	S set -g window-size manual
	until_ 5 on term || {
		echo "no client attached to term" >&2
		exit 2
	}
	T send-keys -t =term: "command -v weave >$W/which" Enter
	until_ 5 test -s "$W/which"
	if [ "$(cat "$W/which")" != "$W/bin/weave" ]; then
		echo "term runs $(cat "$W/which"), not $W/bin/weave" >&2
		exit 2
	fi
}

# fresh: no session but term, the client on it at full size, a new
# shell in it, the sample configs as written and no settings.
fresh() {
	local s
	AT==term:
	for s in $(T list-sessions -F '#{session_name}'); do
		[ "$s" = term ] || T kill-session -t "=$s"
	done
	write_projects
	S resize-window -t screen -x $COLS -y $ROWS
	wait_on term || return
	T respawn-pane -k -t =term: bash --norc --noprofile
	wait_runs =term: bash
}

# started <project>: the project's session, started from term.
started() {
	run "weave $1 --start" || return
	until_ 10 T has-session -t "=$1" 2>/dev/null || fail "$1 didn't start"
}

# logs_idle: shop's logs pane, in LOGS, its sleep 3 exited.
logs_idle() {
	LOGS=$(pane_of shop:api logs)
	[ -n "$LOGS" ] || fail "shop has no logs pane" || return
	wait_runs "$LOGS" bash 8
}

settings() { printf '%s\n' "$@" >"$W/cfg/weaver/settings.yml"; }
config() { cat "$W/cfg/weaver/projects/$1.yml"; }

clean() {
	T kill-server 2>/dev/null
	S kill-server 2>/dev/null
	rm -rf "$W"
}

# --- Scenarios: each starts from fresh --------------------------------------

s1() {
	browse || return
	local want
	for want in '─ projects ─' '─ tasks ─' '─ blog.yml ─' '  notes' '  shop' 'q quit'; do
		shows "$want" || fail "'$want' isn't drawn" || return
	done
}

s2() {
	browse blog || return
	keys Enter
	wait_on blog || return
	expect "the active pane" "$(T display -p -t =blog: '#{@label}')" preview || return
	# 160x44, less a column of borders and the pane names' top borders:
	# 60/40 of the columns, 70/30 of the rows.
	expect "the grid" "$(T list-panes -t =blog:write -F '#{@label}=#{pane_width}x#{pane_height}' | tr '\n' ' ')" \
		"editor=95x43 preview=64x29 shell=64x13 "
}

s3() {
	run 'weave shop' || return
	wait_on shop 10 || return
	expect "the window and pane" "$(T display -p -t =shop: '#{window_name} #{@label}')" "api back" || return
	logs_idle || return
	local back
	back=$(pane_of shop:api back)
	T send-keys -t "$LOGS" 'echo "env=$APP_ENV $LOG"' Enter
	T send-keys -t "$back" 'echo "env=$APP_ENV $LOG"' Enter
	AT=$LOGS wait_for 'env=staging info' || return
	AT=$back wait_for 'env=dev info' || return
	wait_for 'setting up shop'
}

s4() {
	started shop || return
	logs_idle || return
	browse shop || return
	keys Right
	wait_for '⚠ window "api", pane "logs": command exited: sleep 3' || return
	wait_for "APP_ENV=staging  window api, over project's local" || return
	wait_for 'r rerun' || return
	keys Tab
	wait_gone 'r rerun'
}

s5() {
	started shop || return
	logs_idle || return
	browse shop || return
	keys t Enter
	wait_on shop || return
	expect "the active pane" "$(T display -p -t =shop: '#{@label}')" logs || return
	wait_runs "$LOGS" sleep || return
	T switch-client -t =term:
	wait_on term || return
	browse shop || return
	wait_for '● wait'
}

s6() {
	started shop || return
	logs_idle || return
	T send-keys -t "$LOGS" 'sleep 300' Enter
	wait_runs "$LOGS" sleep || return
	browse shop || return
	keys t Down Enter
	wait_for 'logs is busy' || return
	keys y
	wait_on shop || return
	until_ 5 T has-session -t =shop:echo 2>/dev/null || fail "no window echo" || return
	expect "echo's options" "$(T display -p -t =shop:echo '#{@weaver_throwaway} #{@weaver_task_of}')" "1 $LOGS" || return
	until_ 5 has_line =shop:echo hi || fail "echo never printed hi"
}

s7() {
	started shop || return
	logs_idle || return
	browse shop || return
	keys t r
	wait_for '● wait' || return
	wait_runs "$LOGS" sleep || return
	on term || fail "r left the browser" || return
	T send-keys -t "$LOGS" C-c
	wait_gone '● wait' || return
	shows '  wait' || fail "wait left the task list"
}

s8() {
	started shop || return
	browse shop || return
	keys y
	wait_for 'setup: [echo setting up shop]' || return
	keys y
	wait_for 'shop.yml · preview' || return
	wait_gone 'setup: [echo setting up shop]'
}

s9() {
	started shop || return
	logs_idle || return
	browse shop || return
	keys Right
	wait_for 'r rerun' || return
	keys r
	wait_on shop || return
	expect "the active pane" "$(T display -p -t =shop: '#{@label}')" logs || return
	wait_runs "$LOGS" sleep
}

s10() {
	form 'weave edit shop' || return
	cursor_to focus || return
	keys Enter C-s
	wait_for 'Save these changes to the config?' || return
	local s
	s=$(screen)
	expect "focus lines the diff removes" "$(grep -cE '^ - +focus: true' <<<"$s")" 1 || return
	expect "focus lines the diff keeps" "$(grep -cE '^   +focus: true' <<<"$s")" 1 || return
	keys y
	wait_for 'Saved shop.' || return
	expect "focus lines" "$(config shop | grep -c 'focus: true')" 1 || return
	expect "the focused pane" "$(config shop | grep -B1 'focus: true' | head -1 | tr -d ' ')" "-name:back"
}

s11() {
	started blog || return
	form 'weave bind blog' || return
	keys C-s
	wait_for "Replace blog's registered config with this?" || return
	expect "questions asked" "$(screen | grep -c "Replace blog's")" 1 || return
	keys n
	wait_for 'blog is already registered' || return
	keys C-u journal Enter C-s
	wait_for "Save the new project's config?" || return
	keys y
	until_ 5 test -e "$W/cfg/weaver/projects/journal.yml" || fail "journal.yml wasn't written" || return
	expect "the windows" "$(config journal | grep -E '^  - name:' | tr -d ' ')" "-name:write" || return
	expect "the focused pane" "$(config journal | grep -B1 'focus: true' | head -1 | tr -d ' ')" "-name:preview"
}

s12() {
	started shop || return
	AT==shop:shell
	browse shop || return
	keys x
	wait_for 'Kill' || return
	keys y
	wait_for 'tearing down' || return
	wait_for 'teardown: false: command failed' || return
	wait_for "shop's session will be killed anyway, with this pane." || return
	wait_for 'Press Enter to go on.' || return
	keys Enter
	until_ 5 ended shop || fail "shop is still running"
}

s13() {
	run 'weave notes' || return
	wait_on notes 10 || return
	expect "the window" "$(T display -p -t =notes: '#{window_name}')" code || return
	expect "the window size" "$(T display -p -t =notes:code '#{window_width}x#{window_height}')" 160x44 || return
	expect "the widths" "$(T list-panes -t =notes:code -F '#{@label}=#{pane_width}' | tr '\n' ' ')" "files=30 edit=129 "
}

s14() {
	browse notes || return
	wait_for '┌──┬─ edit' || return
	S resize-window -t screen -y 20
	until_ 5 names_only || fail "not the window names alone, at 20 lines"
}

# names_only: the window list without its miniatures, both checked on
# one screen, as a resize draws a few half-made ones first.
names_only() {
	local s
	s=$(screen)
	! grep -qF '┌──┬─ edit' <<<"$s" && grep -qE ' code +shell ' <<<"$s"
}

s15() {
	settings 'selected-colour: 214' 'keys: { split-right: "|", split-below: "-" }'
	form 'weave edit notes' || return
	test -e "$W/cfg/weaver/settings.schema.json" || fail "no settings.schema.json" || return
	cursor_to 'code +shell' || return
	keys Right
	wait_for 'notes › window shell' || return
	local i
	for ((i = 0; i < 20; i++)); do
		shows '┏' && break
		step Down || return
	done
	wait_for '| split right · - split below' || return
	keys '|'
	wait_for 'pane 1   pane 2' || return
	T capture-pane -p -e -t "$AT" | grep -qF '38;5;214' || fail "nothing in colour 214" || return
	keys u Escape
	wait_for 'Nothing saved.'
}

s16() {
	settings 'keys: { remove: u }'
	run 'weave edit --raw notes' || return
	wait_for 'Error: /tmp/wvt/cfg/weaver/settings.yml: keys.remove: key taken: "u" already does undo' || return
	run 'weave list | cut -d" " -f1 | paste -sd" "' || return
	until_ 5 has_line "$AT" 'blog notes shop' || fail "weave list didn't list the projects" || return
	run 'weave --version >/dev/null && weave completion bash >/dev/null && echo both-ran' || return
	until_ 5 has_line "$AT" both-ran || fail "--version or completion failed"
}

s17() {
	form 'weave edit notes' || return
	cursor_to areas || return
	keys Enter C-u
	keys -l 'files, edit / files, logs'
	wait_for '─ logs ─' || return
	keys Enter C-s
	wait_for 'Save these changes to the config?' || return
	wait_for '- [files, logs]' || return
	wait_for '- name: logs' || return
	keys y
	wait_for 'Saved notes.' || return
	config notes | grep -qF 'columns: [30, 1fr]' || fail "the columns changed" || return
	expect "the panes" "$(config notes | grep -oE 'name: (files|edit|logs)' | tr '\n' ' ')" \
		"name: files name: edit name: logs "
}

s18() {
	form 'weave edit notes' || return
	cursor_to layout || return
	local before size
	before=$(split_at)
	keys Right
	wait_gone ' areas ' || return
	expect "the column edit starts at" "$(split_at)" "$before" || return
	keys C-s
	wait_for 'Save these changes to the config?' || return
	wait_for 'grid:' || return
	keys y
	wait_for 'Saved notes.' || return
	size=$(config notes | grep -A4 'name: edit' | grep -oE 'size: [0-9]+%' | grep -oE '[0-9]+')
	# edit is 129 of term's 160 columns; tmux's default 80 would make it
	# 49 of 80, 61%.
	[ -n "$size" ] && [ "$size" -ge 78 ] && [ "$size" -le 84 ] || fail "edit's split is '${size}%', not about 81%"
}

s19() {
	started notes || return
	browse notes || return
	wait_for 'notes.yml · preview' || return
	T send-keys -t =notes:code.0 "printf '\\033[32mgreen\\033[0m\\n'" Enter
	T split-window -d -t =notes:code
	wait_for 'green' || return
	T capture-pane -p -e -t "$AT" | grep -qF '32mgreen' || fail "green isn't green" || return
	keys Tab Tab
	wait_for 'not in the config' || return
	keys p
	wait_gone '· preview' || return
	wait_gone 'not in the config' || return
	keys p
	wait_for '· preview' || return
	T kill-session -t =notes
	wait_for "preview: notes's session ended"
}

s20() {
	run 'weave notes' || return
	wait_on notes 10 || return
	AT==notes:code
	T resize-pane -Z -t "$AT"
	browse notes || return
	wait_for 'this browser'
}

# --- Running ----------------------------------------------------------------

main() {
	local bin= keep= n all=() pass=0 failed=()
	while getopts b:k opt; do
		case $opt in
		b) bin=$(cd "$OPTARG" && pwd) || exit 2 ;;
		k) keep=1 ;;
		*) exit 2 ;;
		esac
	done
	shift $((OPTIND - 1))
	cd "$(dirname "$0")/../.." || exit 2
	unset TMUX
	if [ $# -gt 0 ]; then
		all=("$@")
	else
		for n in $(declare -F | awk '$3 ~ /^s[0-9]+$/ {print substr($3, 2)}' | sort -n); do all+=("$n"); done
	fi
	[ -e "$W" ] && {
		echo "$W exists, from an earlier run: remove it first" >&2
		exit 2
	}
	# Cleaned up however the run ends, unless kept.
	[ -n "$keep" ] || trap clean EXIT
	setup "$bin"
	for n in "${all[@]}"; do
		FAILED=
		if fresh && "s$n" && [ -z "$FAILED" ]; then
			echo "PASS $n"
			pass=$((pass + 1))
		else
			echo "FAIL $n: ${FAILED:-an error}"
			# The screen less its blank rows, those of borders only included.
			screen | sed -E 's/ +$//' | grep -vE '^[│┃ ]*$' | sed 's/^/  | /'
			failed+=("$n")
		fi
	done
	echo "$pass/${#all[@]} passed${failed:+, failed: ${failed[*]}}"
	[ ${#failed[@]} -eq 0 ]
}

# Sourced, it only defines the functions, to drive a kept run by hand.
if [ "${BASH_SOURCE[0]}" = "$0" ]; then
	main "$@"
fi
