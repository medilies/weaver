package main

import (
	"github.com/spf13/cobra"

	"github.com/medilies/weaver/internal/session/domain"
)

func newShowCommand(weaver application) *cobra.Command {
	return &cobra.Command{
		Use:               "show <project>",
		Short:             "Print a registered project's config file",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeProjects(weaver),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := domain.NewProjectName(args[0])
			if err != nil {
				return err
			}
			document, err := weaver.documents.Document(name)
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(document.Content)
			return err
		},
	}
}
