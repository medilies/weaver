package main

import "github.com/spf13/cobra"

func newKillCommand(weaver application) *cobra.Command {
	return &cobra.Command{
		Use:               "kill <project>",
		Short:             "Kill a project's tmux session and everything running in it",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeProjects(weaver),
		RunE: func(cmd *cobra.Command, args []string) error {
			return weaver.killProject.Handle(args[0])
		},
	}
}
