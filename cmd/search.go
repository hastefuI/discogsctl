package cmd

import (
	"context"
	"strings"

	"github.com/spf13/cobra"
	"go.hasteful.org/discogsctl/api"
	"go.hasteful.org/discogsctl/internal/cli"
)

func newSearchCmd(cfg *config) *cobra.Command {
	var q api.SearchQuery
	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search the Discogs database",
		Example: `  discogsctl search nevermind --type master
  discogsctl search --type release --artist "Nine Inch Nails" --year 1994 --output json`,
	}

	f := cmd.Flags()
	f.StringVar(&q.Type, "type", "", "resource type: "+strings.Join(api.SearchTypes, ", "))
	f.StringVar(&q.Title, "title", "", `combined "Artist - Release Title" field`)
	f.StringVar(&q.ReleaseTitle, "release-title", "", "release title")
	f.StringVar(&q.Credit, "credit", "", "release credit")
	f.StringVar(&q.Artist, "artist", "", "artist name")
	f.StringVar(&q.ANV, "anv", "", "artist name variation")
	f.StringVar(&q.Label, "label", "", "label name")
	f.StringVar(&q.Genre, "genre", "", "genre")
	f.StringVar(&q.Style, "style", "", "style")
	f.StringVar(&q.Country, "country", "", "release country")
	f.StringVar(&q.Year, "year", "", "release year")
	f.StringVar(&q.Format, "format", "", "format")
	f.StringVar(&q.Catno, "catno", "", "catalogue number")
	f.StringVar(&q.Barcode, "barcode", "", "barcode")
	f.StringVar(&q.Track, "track", "", "track title")
	f.StringVar(&q.Submitter, "submitter", "", "submitter username")
	f.StringVar(&q.Contributor, "contributor", "", "contributor username")
	pages := cli.BindPageFlags(f)

	cmd.RunE = listRun(cfg, pages, func(ctx context.Context, args []string, page api.Page) (*api.Paginated[api.SearchResult], error) {
		q.Query = strings.Join(args, " ")
		q.Page = page
		return cfg.client.Search(ctx, q)
	})
	return cmd
}
