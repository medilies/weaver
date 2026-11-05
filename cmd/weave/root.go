package main

import (
	"os"

	"github.com/spf13/cobra"
)

func newRootCommand(weaver application) *cobra.Command {
	root := &cobra.Command{
		Use:   "weave [project | .]",
		Short: "Start or attach a project's tmux session",
		Long: `weave <project> attaches to the project's tmux session, starting it
first from ~/.config/weaver/projects/<project>.yml when it
isn't running. Inside tmux, it switches to the session instead.

weave . opens the project whose root contains the current directory.

Without a project, weave lists the projects and the selected one's
config, to open, add, edit, kill or delete them.`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeProjects(weaver),
		SilenceUsage:      true,
		RunE: func(_ *cobra.Command, args []string) error {
			// "." can't be a project name, which starts with a letter or digit.
			if len(args) == 1 && args[0] != "." {
				return weaver.openProject.Handle(args[0])
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
	root.AddCommand(
		newFreezeCommand(weaver),
		newEditCommand(weaver),
		newKillCommand(weaver),
		newListCommand(weaver),
		newShowCommand(weaver),
	)
	return root
}
