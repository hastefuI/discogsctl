package cmd

import "github.com/spf13/cobra"

func newWhoamiCmd(cfg *config) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the user the token belongs to",
		Long:  "Show the user the token belongs to. It is the quickest check that authentication works.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := cfg.client.Identity(cmd.Context())
			if err != nil {
				return err
			}
			return cfg.print(cmd, id)
		},
	}
}
