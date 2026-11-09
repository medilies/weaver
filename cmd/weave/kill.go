package main

import "github.com/spf13/cobra"

func newKillCommand(weaver application) *cobra.Command {
	return &cobra.Command{
		Use:   "kill [project]",
		Short: "Kill a project's tmux session and everything running in it",
		Long: `weave kill ends a project's tmux session and everything running in
it. Without a project, it kills the session weave runs in, if weave
started it.

Before killing a session weave started, it runs the config's teardown
commands in the root; the session is killed even when one fails. Run
inside that session, weave shows the failure and waits for Enter first,
as the pane goes with the session.`,
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
