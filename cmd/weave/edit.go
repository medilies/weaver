package main

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/medilies/weaver/internal/session/app"
)

func newEditCommand(weaver application) *cobra.Command {
	var raw bool
	cmd := &cobra.Command{
		Use:   "edit <project>",
		Short: "Change a project's config in the config form, or raw in your editor",
		Long: `weave edit opens the config form: the project, its windows and their
panes as cards, each field checked as you type it. ctrl+s saves, esc
leaves. Saving rewrites the config, so its comments go: weaver asks
first when it has some.

weave edit --raw opens a copy of the config in $VISUAL, else $EDITOR,
else vi (with VS Code, use VISUAL="code --wait"), and keeps its
comments. The config is replaced only once the copy is valid: on a
mistake, weaver asks whether to discard your changes, and otherwise
reopens the editor on what you wrote.`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeProjects(weaver),
		RunE: func(cmd *cobra.Command, args []string) error {
			changed := true
			var err error
			if raw {
				changed, err = weaver.editProject.HandleRaw(args[0])
			} else {
				err = weaver.editProject.Handle(args[0])
			}
			if errors.Is(err, app.ErrAborted) {
				_, err = fmt.Fprintln(cmd.ErrOrStderr(), "Nothing saved.")
				return err
			}
			if err != nil {
				return err
			}
			message := "No changes."
			if changed {
				message = "Saved " + args[0] + "."
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), message)
			return err
		},
	}
	cmd.Flags().BoolVar(&raw, "raw", false, "edit the config in your editor, keeping its comments")
	return cmd
}
