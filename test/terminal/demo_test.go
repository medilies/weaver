//go:build terminal

package terminal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tuitest "github.com/microsoft/tui-test/bindings/go"
)

// The look of the demo's tmux: Catppuccin Mocha, the status line on top,
// windows and panes numbered from 1.
const demoTmux = `
set -as terminal-features ",*:RGB"
set -g base-index 1
setw -g pane-base-index 1
set -g status-position top
set -g status-justify left
set -g status-style "bg=#1e1e2e,fg=#cdd6f4"
set -g status-left ""
set -g status-right "#[bg=#313244,fg=#cba6f7,bold] #S "
set -g window-status-separator " "
set -g window-status-format "#[bg=#313244,fg=#cdd6f4] #W #[bg=#45475a] #I "
set -g window-status-current-format "#[bg=#cba6f7,fg=#11111b,bold] #W #[bg=#f5c2e7] #I "
set -g window-style "bg=#1e1e2e,fg=#cdd6f4"
set -g window-active-style "bg=#1e1e2e,fg=#cdd6f4"
set -g pane-border-style "fg=#6c7086"
set -g pane-active-border-style "fg=#cba6f7"
set -g message-style "bg=#1e1e2e,fg=#94e2d5"
`

// The programs of the demo's project, made up.
var demoTools = map[string]string{
	"task": `case $1 in
dev) while :; do printf '%s GET /health 200\n' "$(date +%T)"; sleep 1; done ;;
migrate) echo "applying 3 migrations"; sleep 1; echo "done" ;;
esac
`,
	"nimbus": `case $1 in
psql) printf 'psql (16.2)\nnimbus=# '; exec sleep 600 ;;
app) echo "Launching lib/main.dart on Pixel 8"; exec sleep 600 ;;
esac
`,
}

// demoNimbus is the README's nimbus config, so that the demo shows what
// the README says.
func demoNimbus(t *testing.T) string {
	t.Helper()
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	_, config, found := strings.Cut(string(readme), "```yaml\n# ~/.config/weaver/projects/nimbus.yml\n")
	config, _, closed := strings.Cut(config, "```")
	if !found || !closed {
		t.Fatal("the README shows no nimbus.yml")
	}
	return config
}

const demoSite = `root: ~/dev/site
windows:
  - name: edit
  - name: serve
`

// TestDemo: with -demo FILE only. `weave`, the windows and panes of
// `nimbus` with ← → and Tab, `s`, then its tasks with `t` and `r`, its
// config with `e`, and Enter.
//
// FILE is the GIF the README shows: the screen of a 112x30 terminal in
// Catppuccin Mocha, mauve the selected colour, above a caption naming
// what each step shows. ffmpeg crops the recorder's window frame off,
// adds the captions and shrinks it. It is paced for eyes: this is the one
// place a test sleeps, to let the viewer read, never to wait for weave.
func TestDemo(t *testing.T) {
	if *demo == "" {
		t.Skip("not asked for: -demo FILE")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal("the demo needs ffmpeg")
	}
	bold, regular := demoFont(t, "Noto Sans:bold"), demoFont(t, "Noto Sans")
	h := startIn(t, world{cols: 112, rows: 30, tmuxConf: demoTmux, tools: demoTools})
	samples, _ := filepath.Glob(filepath.Join(h.dir, "cfg/weaver/projects/*.yml"))
	for _, sample := range samples {
		if err := os.Remove(sample); err != nil {
			t.Fatal(err)
		}
	}
	h.settings(`selected-colour: "#cba6f7"`)
	h.write("cfg/weaver/projects/nimbus.yml", demoNimbus(t))
	h.write("cfg/weaver/projects/site.yml", demoSite)
	for _, d := range []string{"home/dev/nimbus/api", "home/dev/nimbus/app", "home/dev/site"} {
		h.mkdir(d)
	}

	h.typed("export PS1='$ '; clear")
	h.promptIn("term")
	recording := filepath.Join(h.dir, "recording.gif")
	if err := h.screen.StartRecording(recording, tuitest.RecordingOptions{Zoom: tuitest.Ptr(0.45)}); err != nil {
		t.Fatalf("recording: %v", err)
	}
	began := time.Now()
	var captions []caption
	captioned := func(label, text string) {
		captions = append(captions, caption{at: time.Since(began), label: label, text: text})
	}
	beat := func(seconds float64) { time.Sleep(time.Duration(seconds * float64(time.Second))) }
	slowly := func(text string) {
		for _, r := range text {
			h.types(string(r))
			beat(0.07)
		}
	}

	captioned("browse", "weave alone lists your projects")
	beat(1)
	slowly("weave")
	beat(0.4)
	h.keys("Enter")
	h.sees("enter open")
	beat(2)

	captioned("drawing", "each window and pane, as it will open")
	h.keys("Right")
	beat(1.2)
	h.keys("Right")
	beat(1.2)
	h.keys("Left", "Left")
	beat(1)
	h.keys("Tab")
	beat(1)
	h.keys("Tab")
	beat(1.5)

	captioned("start", "the session runs, its preview kept live")
	h.keys("s")
	h.eventually("nimbus starts", func() bool { return h.running("nimbus") })
	h.sees("· preview")
	beat(3.5)

	captioned("tasks", "run one in its pane, staying in the browser")
	h.keys("t")
	beat(1.2)
	h.keys("r")
	h.sees("applying 3 migrations")
	beat(3)
	h.keys("Escape")
	beat(0.8)

	captioned("config form", "change the project, each field checked as typed")
	h.keys("e")
	h.sees("api shell")
	beat(3)
	h.keys("Escape")
	h.sees("enter open")
	captioned("open", "enter puts you in the session")
	beat(1)
	h.keys("Enter")
	h.eventually("the client moves to nimbus", func() bool { return h.clientOn() == "nimbus" })
	beat(4)

	if _, err := h.screen.StopRecording(); err != nil {
		t.Fatalf("recording: %v", err)
	}
	finish := exec.Command(ffmpeg, "-v", "error", "-y", "-i", recording, "-vf", demoFilter(captions, bold, regular), *demo)
	if out, err := finish.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, out)
	}
}

// caption is what the GIF says from a moment of the recording on.
type caption struct {
	at          time.Duration
	label, text string
}

// demoFilter is ffmpeg's filter making the recording the README's GIF: the
// terminal's cells cropped out of the recorder's window frame, as they sit
// in a 112x30 terminal at a zoom of 0.45, a margin in Mocha's background,
// a caption band in Latte's, light so that it can't pass for part of
// weave's screen, and the colours cut down to keep it small.
func demoFilter(captions []caption, bold, regular string) string {
	filters := []string{
		"crop=1008:567:36:59",
		"pad=iw+24:ih+24+52:12:12:color=0x1e1e2e",
		"drawbox=x=0:y=ih-52:w=iw:h=52:color=0xeff1f5:t=fill",
	}
	for i, c := range captions {
		enable := fmt.Sprintf("gte(t,%.2f)", c.at.Seconds())
		if i+1 < len(captions) {
			enable = fmt.Sprintf("between(t,%.2f,%.2f)", c.at.Seconds(), captions[i+1].at.Seconds())
		}
		filters = append(filters,
			fmt.Sprintf("drawtext=fontfile='%s':text='%s':fontsize=20:fontcolor=0x1e1e2e:x=24:y=h-36:enable='%s'",
				bold, strings.ToUpper(c.label), enable),
			fmt.Sprintf("drawtext=fontfile='%s':text='%s':fontsize=20:fontcolor=0x5c5f77:x=200:y=h-36:enable='%s'",
				regular, c.text, enable),
		)
	}
	filters = append(filters, "split[a][b];[a]palettegen=max_colors=128:stats_mode=diff[p];[b][p]paletteuse=dither=none:diff_mode=rectangle")
	return strings.Join(filters, ",")
}

// demoFont is the file of the font fontconfig matches to pattern.
func demoFont(t *testing.T, pattern string) string {
	t.Helper()
	file, err := exec.Command("fc-match", "-f", "%{file}", pattern).Output()
	if err != nil || len(file) == 0 {
		t.Fatalf("the demo needs fontconfig's fc-match, finding %q: %v", pattern, err)
	}
	return string(file)
}

// mkdir makes a directory under the test's.
func (h *harness) mkdir(name string) {
	h.t.Helper()
	if err := os.MkdirAll(filepath.Join(h.dir, name), 0o700); err != nil {
		h.t.Fatal(err)
	}
}
