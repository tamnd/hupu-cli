package cli

import (
	"github.com/spf13/cobra"
)

// hotCmd returns the hot command.
func (a *App) hotCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "hot",
		Short: "List trending posts from the Hupu BBS homepage",
		RunE: func(cmd *cobra.Command, _ []string) error {
			n := a.effectiveLimit(20)
			a.progressf("fetching %d hot posts from Hupu BBS...", n)
			posts, err := a.client.Hot(cmd.Context(), n)
			if err != nil {
				return codeError(exitError, err)
			}
			return a.renderRecords(posts, len(posts))
		},
	}
}
