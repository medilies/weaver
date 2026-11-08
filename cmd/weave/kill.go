package main

import "github.com/spf13/cobra"

func newKillCommand(weaver application) *cobra.Command {
	return &cobra.Command{
		Use:   "kill [project]",
		Short: "Kill a project's tmux session and everything running in it",
		Long: `weave kill ends a project's tmux session and everything running in
it. Without a project, it kills the session weave runs in, if weave
started it.`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeProjects(weaver),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return weaver.killProject.HandleCurrent()
			}
			return weaver.killProject.Handle(args[0])
		},
	}
}
