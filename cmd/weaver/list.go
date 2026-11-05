package main

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newListCommand(weaver application) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered projects and whether their session runs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			statuses, err := weaver.listProjects.Handle()
			if err != nil {
				return err
			}
			table := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			for _, status := range statuses {
				state := "-"
				if status.Running() {
					state = "running"
				}
				if _, err := fmt.Fprintf(table, "%s\t%s\n", status.Name(), state); err != nil {
					return err
				}
			}
			return table.Flush()
		},
	}
}
