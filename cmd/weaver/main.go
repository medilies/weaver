// Command weaver drives tmux sessions, windows and panes from a
// per-project JSON config.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/infra"
)

func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "weaver", "projects"), nil
}

func newShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <project>",
		Short: "Show a registered project's config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := configDir()
			if err != nil {
				return err
			}

			useCase := app.NewLoadProject(infra.NewJSONProjectRepository(dir))
			project, err := useCase.Handle(args[0])
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), project.Describe())
			return err
		},
	}
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "weaver",
		Short: "Weaver drives tmux sessions from per-project JSON config",
	}
	root.AddCommand(newShowCommand())
	return root
}

func main() {
	if err := newRootCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
