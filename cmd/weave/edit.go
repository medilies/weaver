package main

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/medilies/weaver/internal/session/app"
)

func newEditCommand(weaver application) *cobra.Command {
	return &cobra.Command{
		Use:   "edit <project>",
		Short: "Edit a project's config in your editor, keeping only a valid one",
		Long: `weave edit opens a copy of the project's config in $VISUAL, else
$EDITOR, else vi (with VS Code, use VISUAL="code --wait"). The config is
replaced only once the copy is valid: on a mistake, weaver asks whether
to discard your changes, and otherwise reopens the editor on what you
wrote.`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeProjects(weaver),
		RunE: func(cmd *cobra.Command, args []string) error {
			changed, err := weaver.editProject.Handle(args[0])
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
}
