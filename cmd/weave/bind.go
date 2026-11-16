package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/medilies/weaver/internal/session/app"
)

func newBindCommand(weaver application) *cobra.Command {
	var name, config string
	cmd := &cobra.Command{
		Use:   "bind [session]",
		Short: "Register the current directory as a project's root, through the config form",
		Long: `weave bind opens the config form on the project rooted at the current
directory: its config, weaver.yml unless --config names another, when it
has one, else a new project named after the directory, with one window.
Saving writes the config and lists the directory under projects: in
settings.yml, first.

Given a running tmux session, the windows and panes start as the session
has them: lay them out by hand, then let weaver record them. "." is the
session weave runs in. The command in each pane's foreground is kept, to
be typed but not run: check it, as pipes, quotes and variables don't
survive. Set the pane to start "command" to run it. Once saved, the
session is renamed after the project, so it counts as the project's.`,
		Args: cobra.MaximumNArgs(1),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			sessions, err := weaver.sessions.Running()
			if err != nil {
				return nil, cobra.ShellCompDirectiveError
			}
			names := make([]cobra.Completion, 0, len(sessions))
			for _, session := range sessions {
				names = append(names, session.Name())
			}
			return names, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
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
			request := app.BindRequest{Name: name, Config: config, WorkingDir: dir}
			if len(args) == 1 {
				request.Session = args[0]
			}
			project, err := weaver.bindProject.Handle(request)
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
	cmd.Flags().StringVar(&name, "name", "", "the project's name (default: its config's, else the directory's)")
	cmd.Flags().StringVar(&config, "config", "", "the config to write, relative to the directory or absolute (default: weaver.yml)")
	return cmd
}
