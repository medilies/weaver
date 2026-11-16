package main

import (
	"os"

	"github.com/spf13/cobra"
)

func newKillCommand(weaver application) *cobra.Command {
	return &cobra.Command{
		Use:   "kill [project | dir | .]",
		Short: "Kill a project's tmux session and everything running in it",
		Long: `weave kill ends a project's tmux session and everything running in
it. A project is named as weave names it. Without a project, it kills
the session weave runs in, if weave started it.

Before killing a session weave started, it types ctrl+c into each of
its busy panes and waits, up to 5 seconds, for them to be back at their
prompts; then it runs the config's teardown commands in the root. The
session is killed even when a pane still runs or a teardown command
fails. Run inside that session, weave shows the failure and waits for
Enter first, as the pane goes with the session.`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeProjects(weaver),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return weaver.killProject.HandleCurrent()
			}
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			project, err := weaver.findProject.Handle(args[0], dir)
			if err != nil {
				return err
			}
			lingering, err := weaver.killProject.Handle(project.Root())
			if lingering != "" {
				cmd.PrintErrln(lingering)
			}
			return err
		},
	}
}
