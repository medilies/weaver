package main

import "github.com/spf13/cobra"

func newRootCommand(weaver application) *cobra.Command {
	root := &cobra.Command{
		Use:   "weaver [project]",
		Short: "Start or attach a project's tmux session",
		Long: `weaver <project> attaches to the project's tmux session, starting it
first from ~/.config/weaver/projects/<project>.json when it isn't
running. Inside tmux, it switches to the session instead.`,
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return weaver.openProject.Handle(args[0])
		},
	}
	root.AddCommand(newShowCommand(weaver))
	return root
}
