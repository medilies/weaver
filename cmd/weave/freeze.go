package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/infra"
)

func newFreezeCommand(weaver application) *cobra.Command {
	var name, root string
	cmd := &cobra.Command{
		Use:   "freeze [session]",
		Short: "Register a project through interactive forms, from a running session if given",
		Long: `weave freeze asks for a project's name and root (the current
directory unless --root says otherwise), the repositories expected under
it, the before commands of its panes, and its windows and panes,
then saves it.

Given a running tmux session, the windows and panes start as the session
has them, and the name as the session's: lay them out by hand, then let
weaver record them. "." is the session weave runs in. The command in each
pane's foreground is kept, to be typed but not run: check it, as pipes,
quotes and variables don't survive. Choose "Run it" to run it.`,
		Args: cobra.MaximumNArgs(1),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			sessions, err := weaver.sessions.Running()
			if err != nil {
				return nil, cobra.ShellCompDirectiveError
			}
			return sessions, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			// The shell leaves the ~ of --root=~/dev/shop as typed. As in a
			// config, ~user isn't expanded, nor taken for a relative path.
			expanded, err := infra.ExpandHome(root)
			if err != nil {
				return err
			}
			if strings.HasPrefix(expanded, "~") {
				return fmt.Errorf("--root %s: only a leading ~/ stands for your home directory", root)
			}
			request := app.FreezeRequest{Name: name, Root: expanded, WorkingDir: dir}
			if len(args) == 1 {
				request.Session = args[0]
			}
			project, err := weaver.freezeProject.Handle(request)
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
	cmd.Flags().StringVar(&name, "name", "", "the project's name (default: the session's, else the root's base name)")
	cmd.Flags().StringVar(&root, "root", "", "the project's directory (default: the current directory)")
	_ = cmd.MarkFlagDirname("root")
	return cmd
}
