package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newRootCommand(weaver application) *cobra.Command {
	var start bool
	root := &cobra.Command{
		Use:   "weave [project | .]",
		Short: "Open a project's tmux session",
		Long: `weave <project> attaches to the project's tmux session, creating it
first from ~/.config/weaver/projects/<project>.yml when it
isn't running, after running the config's setup commands in the root,
stopping if one fails. Inside tmux, it switches to the session instead.

weave . opens the project whose root contains the current directory.

With --start, weave creates the session, or restores the windows it
lacks, without attaching or switching, and says what it did.

Without a project, weave lists the projects and the selected one's
summary and config, to open, start, bind, edit, kill or delete them, or
run their tasks.`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeProjects(weaver),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if start {
				return startProject(cmd, weaver, args)
			}
			// "." can't be a project name, which starts with a letter or digit.
			if len(args) == 1 && args[0] != "." {
				return weaver.openProject.Handle(args[0])
			}
			if len(args) == 0 {
				if err := weaver.readSettings(); err != nil {
					return err
				}
			}
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			if len(args) == 0 {
				return weaver.browseProjects.Handle(dir)
			}
			return weaver.openProject.HandleDir(dir)
		},
	}
	root.Flags().BoolVarP(&start, "start", "s", false, "start the session without attaching or switching to it")
	root.AddCommand(
		newBindCommand(weaver),
		newEditCommand(weaver),
		newKillCommand(weaver),
		newListCommand(weaver),
		newShowCommand(weaver),
	)
	return root
}

// startProject starts the project args name, or whose root contains
// the current directory for ".", and prints what it did.
func startProject(cmd *cobra.Command, weaver application, args []string) error {
	if len(args) == 0 {
		return errors.New("--start needs a project, or . for the current directory's")
	}
	var done string
	var err error
	if args[0] == "." {
		dir, dirErr := os.Getwd()
		if dirErr != nil {
			return dirErr
		}
		done, err = weaver.openProject.HandleDirStart(dir)
	} else {
		done, err = weaver.openProject.HandleStart(args[0])
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), done)
	return err
}
