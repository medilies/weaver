package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/medilies/weaver/internal/session/app"
)

func newAddCommand(weaver application) *cobra.Command {
	var root, from string
	cmd := &cobra.Command{
		Use:   "add [name]",
		Short: "Register a project through interactive forms",
		Long: `weave add asks for a project's root (the current directory unless
--root says otherwise), the repositories to clone when missing, the PHP
and Node versions, and its windows and panes, then saves it.

With --from, the windows and panes start as a running tmux session has
them: lay them out by hand, then let weaver record them.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			request := app.AddRequest{Root: root, WorkingDir: dir, From: from}
			if len(args) == 1 {
				request.Name = args[0]
			}
			project, err := weaver.addProject.Handle(request)
			if errors.Is(err, app.ErrAborted) {
				_, err = fmt.Fprintln(cmd.ErrOrStderr(), "Nothing saved.")
				return err
			}
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Saved %s. Open it with: weave %s\n", project.Name(), project.Name())
			return err
		},
	}
	cmd.Flags().StringVar(&root, "root", "", "the project's directory (default: the current directory)")
	_ = cmd.MarkFlagDirname("root")
	cmd.Flags().StringVar(&from, "from", "", "a running tmux session to start the windows and panes from")
	_ = cmd.RegisterFlagCompletionFunc("from", func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
		sessions, err := weaver.sessions.Running()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		return sessions, cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}
