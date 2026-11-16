package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// openHelp says what opening a project does, for weave and weave open.
const openHelp = `weave <project> attaches to the project's tmux session, creating it
first from its config when it isn't running, after running the config's
setup commands in the root, stopping if one fails. Inside tmux, it
switches to the session instead.

A project is named by its name, or by a directory its root contains: one
holding "/", or "." for the current directory. A name two projects share
is refused; name one by its root instead.

With --detached, weave starts the project: it creates the session, or
restores the windows it lacks, without attaching or switching, and says
what it did.`

func newRootCommand(weaver application) *cobra.Command {
	var detached bool
	root := &cobra.Command{
		Use:   "weave [project | dir | .]",
		Short: "Open a project's tmux session",
		Long: openHelp + `

The projects are the roots listed under projects: in settings.yml, in
~/.config/weaver, or ~/Library/Application Support/weaver on macOS, each
with its config, weaver.yml in the root unless the entry says otherwise.
weave open opens a project named after one of weave's commands.

Without a project, weave lists the projects and the selected one's
summary and config, to open, start, bind, edit, kill or unregister them,
or run their tasks.`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeProjects(weaver),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return openProject(cmd, weaver, args[0], detached)
			}
			if detached {
				return fmt.Errorf("--detached needs a project, or . for the current directory's")
			}
			if err := weaver.readSettings(); err != nil {
				return err
			}
			if popped, err := weaver.inPopup(); popped || err != nil {
				return err
			}
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			return weaver.browseProjects.Handle(dir)
		},
	}
	root.Flags().BoolVarP(&detached, "detached", "d", false, "start the session without attaching or switching to it")
	root.AddCommand(
		newOpenCommand(weaver),
		newBindCommand(weaver),
		newKillCommand(weaver),
	)
	return root
}

func newOpenCommand(weaver application) *cobra.Command {
	var detached bool
	cmd := &cobra.Command{
		Use:               "open <project | dir | .>",
		Short:             "Open a project's tmux session, even one named after a command",
		Long:              openHelp,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeProjects(weaver),
		RunE: func(cmd *cobra.Command, args []string) error {
			return openProject(cmd, weaver, args[0], detached)
		},
	}
	cmd.Flags().BoolVarP(&detached, "detached", "d", false, "start the session without attaching or switching to it")
	return cmd
}

// openProject opens the project arg names, or, detached, starts it and
// prints what it did.
func openProject(cmd *cobra.Command, weaver application, arg string, detached bool) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	project, err := weaver.findProject.Handle(arg, dir)
	if err != nil {
		return err
	}
	if !detached {
		return weaver.openProject.Handle(project.Root())
	}
	done, err := weaver.openProject.HandleStart(project.Root())
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), done)
	return err
}
