package main

import (
	"os"

	"github.com/spf13/cobra"
)

func newRootCommand(weaver application) *cobra.Command {
	root := &cobra.Command{
		Use:   "weaver [project]",
		Short: "Start or attach a project's tmux session",
		Long: `weaver <project> attaches to the project's tmux session, starting it
first from ~/.config/weaver/projects/<project>.json when it isn't
running. Inside tmux, it switches to the session instead.

Without a project, weaver opens the project whose root contains the
current directory.`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeProjects(weaver),
		SilenceUsage:      true,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 1 {
				return weaver.openProject.Handle(args[0])
			}
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			return weaver.openProject.HandleDir(dir)
		},
	}
	root.AddCommand(
		newAddCommand(weaver),
		newEditCommand(weaver),
		newKillCommand(weaver),
		newListCommand(weaver),
		newShowCommand(weaver),
	)
	return root
}
