package cmd

import "github.com/spf13/cobra"

func newUserCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Read user profiles",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "get <username>",
		Short: "Get a user's profile",
		Long:  "Get a user's profile. The email address is shown only to the user themselves.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			u, err := cfg.client.User(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return cfg.print(cmd, u)
		},
	})

	return cmd
}
