package cmd

import "github.com/spf13/cobra"

func newListCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Read user lists",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "get <list_id>",
		Short: "Get a list and its items",
		Long: `Get a user list and every item in it, each a release, master, artist or
label with the owner's comment. List IDs are in user lists. A private list
needs a token for its owner.`,
		Example: "  discogsctl list get 100\n  discogsctl list get 100 --output json | jq -r '.items[] | \"\\(.type) \\(.id) \\(.display_title)\"'",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID("list", args[0])
			if err != nil {
				return err
			}
			l, err := cfg.client.List(cmd.Context(), id)
			if err != nil {
				return err
			}
			return cfg.print(cmd, l)
		},
	})

	return cmd
}
