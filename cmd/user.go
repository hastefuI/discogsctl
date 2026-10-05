package cmd

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"go.hasteful.org/discogsctl/api"
)

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

	cmd.AddCommand(newUserEditCmd(cfg))

	return cmd
}

func newUserEditCmd(cfg *config) *cobra.Command {
	name, homePage, location, profile, currency := new(""), new(""), new(""), new(""), new("")
	cmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit your profile",
		Long: `Edit your Discogs profile. Only the flags given are changed, and an empty
value clears that field, as --location "" does. The profile text takes
Discogs markup, such as [b]bold[/b]. --account-currency sets the currency
your account shows marketplace prices in. The username cannot be changed
through the API. This needs a token.`,
		Example: `  discogsctl user edit --location "Anytown, USA" --home-page https://example.com
  discogsctl user edit --profile "Collecting [b]Detroit techno[/b]"
  discogsctl user edit --account-currency EUR
  discogsctl user edit --location ""`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Only a flag that was given is sent, so "" clears a field and
			// a flag left out keeps it.
			var e api.ProfileEdit
			for flag, field := range map[string]struct{ value, edit **string }{
				"name":             {&name, &e.Name},
				"home-page":        {&homePage, &e.HomePage},
				"location":         {&location, &e.Location},
				"profile":          {&profile, &e.Profile},
				"account-currency": {&currency, &e.Currency},
			} {
				if cmd.Flags().Changed(flag) {
					*field.edit = *field.value
				}
			}
			if e.Currency != nil {
				upper := strings.ToUpper(*e.Currency)
				if !slices.Contains(api.Currencies, upper) {
					return fmt.Errorf("invalid --account-currency %q, want one of %s", *e.Currency, strings.Join(api.Currencies, ", "))
				}
				e.Currency = &upper
			}
			user, err := cfg.tokenHolder(cmd.Context())
			if err != nil {
				return err
			}
			p, err := cfg.client.EditProfile(cmd.Context(), user, e)
			if err != nil {
				return err
			}
			return cfg.print(cmd, p)
		},
	}
	f := cmd.Flags()
	f.StringVar(name, "name", "", "your real name")
	f.StringVar(homePage, "home-page", "", "your website")
	f.StringVar(location, "location", "", "where you are")
	f.StringVar(profile, "profile", "", "your profile text, in Discogs markup")
	f.StringVar(currency, "account-currency", "", "your account's currency for marketplace prices: "+strings.Join(api.Currencies, ", "))
	cmd.MarkFlagsOneRequired("name", "home-page", "location", "profile", "account-currency")
	return cmd
}
