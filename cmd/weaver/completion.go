package main

import "github.com/spf13/cobra"

// completeProjects completes a registered project's name as the first
// argument, noting the projects whose session runs.
func completeProjects(weaver application) cobra.CompletionFunc {
	return func(_ *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		statuses, err := weaver.listProjects.Handle()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		completions := make([]cobra.Completion, 0, len(statuses))
		for _, status := range statuses {
			name := status.Name().String()
			if status.Running() {
				completions = append(completions, cobra.CompletionWithDesc(name, "running"))
			} else {
				completions = append(completions, name)
			}
		}
		return completions, cobra.ShellCompDirectiveNoFileComp
	}
}
