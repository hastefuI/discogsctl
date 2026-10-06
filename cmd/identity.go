package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.hasteful.org/discogsctl/output"
)

func newIdentityCmd(cfg *config) *cobra.Command {
	return &cobra.Command{
		Use:   "identity",
		Short: "Show the identity Discogs reports for the credentials",
		Long: `Show the identity Discogs reports for the token or OAuth access token, from
GET /oauth/identity: the user's ID, username and resource URL, and the name of
the application the credentials were issued to. It is one request, and the
quickest check that authentication works. auth whoami adds the user's profile.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := cfg.client.Identity(cmd.Context())
			if err != nil {
				return err
			}
			return cfg.print(cmd, id)
		},
	}
}

func newWhoamiCmd(cfg *config) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the logged-in user's identity and profile",
		Long: `Show who the token or OAuth access token belongs to: the identity from
GET /oauth/identity, then that user's profile from GET /users/{username}. As
the profile's owner, it includes the email address and private counts.

Text output is one block, the profile with the application name added. JSON
is an object with the two Discogs bodies, under identity and user.`,
		Example: `  discogsctl auth whoami
  discogsctl auth whoami --output json | jq -r '.user.email'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			id, err := cfg.client.Identity(ctx)
			if err != nil {
				return err
			}
			user, err := cfg.client.User(ctx, id.Username)
			if err != nil {
				return fmt.Errorf("getting the profile of %s: %w", id.Username, err)
			}
			return cfg.print(cmd, &output.Whoami{Identity: id, User: user})
		},
	}
}
