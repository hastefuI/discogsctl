package cmd

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"go.hasteful.org/discogsctl/dump"
)

func newDumpCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dump",
		Short: "Work with the Discogs data dumps",
		Long: `Work with the monthly data dumps Discogs publishes at https://data.discogs.com:
XML exports of the artists, labels, masters and releases in the database,
under the CC0 licence. Reading them is not an API request, so no token is
needed and the API rate limit does not apply.`,
	}
	cmd.AddCommand(newDumpListCmd(cfg))
	return cmd
}

func newDumpListCmd(cfg *config) *cobra.Command {
	var (
		year  int
		all   bool
		types []string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the published data dumps",
		Long: `List the data dumps published at data.discogs.com, newest first, with the
types each one has. Not every dump has every type. Without --year or --all,
the newest year is listed.

--type narrows the list to the dumps that have every type named, and each
dump to those files and its checksum. Without it, every dump and file is
listed.

A dump's ID is the date stamp in its file names, such as 20261001. No two
dumps share one, so the ID is how to refer to a dump.

Discogs publishes no machine-readable index, so the list is read from the
download links on the data.discogs.com listing pages. Sizes and times are as
that page shows them: the size is rounded and no time zone is given.`,
		Example: `  discogsctl dump list
  discogsctl dump list --year 2025
  discogsctl dump list --type releases
  discogsctl dump list --all --type masters,releases
  discogsctl dump list --output json | jq -r '.[0].id'
  discogsctl dump list --output json | jq -r '.[] | select(.id == "20261001") | .files[] | select(.type == "releases") | .url'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("year") && year < 1 {
				return fmt.Errorf("--year must be a year such as 2026, got %d", year)
			}
			want, err := dataTypes(types)
			if err != nil {
				return err
			}
			client, err := dump.New(dump.Options{BaseURL: cfg.dumpURL, UserAgent: cfg.agent, Logger: cfg.logger})
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			years := []int{year}
			newest := !all && !cmd.Flags().Changed("year")
			if !cmd.Flags().Changed("year") {
				if years, err = client.Years(ctx); err != nil {
					return err
				}
				if newest {
					years = years[max(0, len(years)-2):]
				}
			}

			// Years is oldest first and List is newest first within a year.
			// Without --year or --all, a year with no dumps of the types
			// asked for, such as early January, falls back to the year
			// before, and no further.
			dumps := []dump.Dump{}
			for _, y := range slices.Backward(years) {
				list, err := client.List(ctx, y)
				if err != nil {
					return err
				}
				for _, d := range list {
					if !d.Has(want...) {
						continue
					}
					if len(want) > 0 {
						d = d.Only(want...)
					}
					dumps = append(dumps, d)
				}
				if newest && len(dumps) > 0 {
					break
				}
			}
			return cfg.print(cmd, dumps)
		},
	}
	cmd.Flags().IntVar(&year, "year", 0, "year to list (default the newest year with dumps)")
	cmd.Flags().BoolVar(&all, "all", false, "list every year, one request per year")
	cmd.Flags().StringSliceVar(&types, "type", nil, "only dumps with every one of these types: "+strings.Join(dump.DataTypes, ", ")+" (default all)")
	cmd.MarkFlagsMutuallyExclusive("year", "all")
	return cmd
}

// dataTypes checks --type against dump.DataTypes, so a typo such as
// "release" is an error rather than an empty list.
func dataTypes(types []string) ([]string, error) {
	out := make([]string, 0, len(types))
	for _, t := range types {
		t = strings.ToLower(strings.TrimSpace(t))
		if !slices.Contains(dump.DataTypes, t) {
			return nil, fmt.Errorf("invalid --type %q, want one of %s", t, strings.Join(dump.DataTypes, ", "))
		}
		out = append(out, t)
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}
