package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newShowCommand(weaver application) *cobra.Command {
	return &cobra.Command{
		Use:   "show <project>",
		Short: "Show a registered project's config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := weaver.loadProject.Handle(args[0])
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), project.Describe())
			return err
		},
	}
}
