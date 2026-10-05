package cmd

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"go.hasteful.org/discogsctl/api"
	"go.hasteful.org/discogsctl/internal/cli"
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

	cmd.AddCommand(newUserEditCmd(cfg), newContributionsCmd(cfg))

	return cmd
}

func newContributionsCmd(cfg *config) *cobra.Command {
	var sort, sortOrder string
	cmd := &cobra.Command{
		Use:   "contributions [username]",
		Short: "List the releases a user has contributed",
		Long: `List the releases a user has contributed to the Discogs database, most
recently added first unless --sort says otherwise. Without a username, they
are the token holder's. Contributions are public, so another user's need no
token.`,
		Example: `  discogsctl user contributions
  discogsctl user contributions <username> --sort year --sort-order desc
  discogsctl user contributions --all --output json | jq -r '.[] | "\(.id) \(.title)"'`,
		Args: cobra.MaximumNArgs(1),
	}
	f := cmd.Flags()
	f.StringVar(&sort, "sort", "", "sort by: "+strings.Join(api.ContributionSorts, ", "))
	f.StringVar(&sortOrder, "sort-order", "", "asc or desc")
	pages := cli.BindPageFlags(f)

	cmd.RunE = listRun(cfg, pages, func(ctx context.Context, args []string, page api.Page) (*api.Paginated[api.Release], error) {
		// The flags are checked before the username is resolved, which can
		// cost a request.
		var q api.ContributionQuery
		var err error
		if q.Sort, err = oneOf("--sort", sort, api.ContributionSorts); err != nil {
			return nil, err
		}
		if q.SortOrder, err = oneOf("--sort-order", sortOrder, []string{"asc", "desc"}); err != nil {
			return nil, err
		}
		user, err := cfg.username(ctx, strings.Join(args, ""))
		if err != nil {
			return nil, err
		}
		q.Page = page
		return cfg.client.Contributions(ctx, user, q)
	})
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
