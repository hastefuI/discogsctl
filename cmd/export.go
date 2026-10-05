package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"go.hasteful.org/discogsctl/api"
	"go.hasteful.org/discogsctl/output"
)

// export is the backup export prints. Each release in the collection names
// its folder, so the folders and the releases together are the whole
// collection.
type export struct {
	Username   string           `json:"username"`
	ExportedAt time.Time        `json:"exported_at"`
	Collection collectionExport `json:"collection"`
	Wantlist   []api.Want       `json:"wantlist"`
}

type collectionExport struct {
	Folders  []api.Folder         `json:"folders"`
	Releases []api.CollectionItem `json:"releases"`
}

func newExportCmd(cfg *config) *cobra.Command {
	var username string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export a collection and wantlist as JSON",
		Long: `Export a user's collection folders, the releases in them and their wantlist,
as one JSON object for backup. Without --username it is the token holder's,
including private folders and notes; another user's export holds only what
they have made public.

The export is always JSON, whatever --output says. Every page is fetched
through the rate limiter, so a large collection takes a while. If any part
fails, nothing is printed, so a redirect never leaves a partial backup that
looks complete.`,
		Example: `  discogsctl export > discogs-backup.json
  discogsctl export --username <username> > their-collection.json
  discogsctl export | jq '{collection: (.collection.releases | length), wantlist: (.wantlist | length)}'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			user, err := cfg.username(ctx, username)
			if err != nil {
				return err
			}
			e := export{Username: user, ExportedAt: time.Now().UTC()}

			if e.Collection.Folders, err = cfg.client.CollectionFolders(ctx, user); err != nil {
				return fmt.Errorf("exporting collection folders: %w", err)
			}
			if e.Collection.Releases, err = every(ctx, func(page api.Page) (*api.Paginated[api.CollectionItem], error) {
				return cfg.client.CollectionItems(ctx, user, api.AllFolder, page)
			}); err != nil {
				return fmt.Errorf("exporting collection: %w", err)
			}
			if e.Wantlist, err = every(ctx, func(page api.Page) (*api.Paginated[api.Want], error) {
				return cfg.client.Wantlist(ctx, user, page)
			}); err != nil {
				return fmt.Errorf("exporting wantlist: %w", err)
			}
			return output.Write(cmd.OutOrStdout(), output.FormatJSON, e)
		},
	}
	cmd.Flags().StringVar(&username, "username", "", usernameHelp)
	return cmd
}

// every fetches every page of a listing, the largest pages Discogs serves, and
// returns all its items.
func every[T any](ctx context.Context, fetch func(api.Page) (*api.Paginated[T], error)) ([]T, error) {
	first, err := fetch(api.Page{Page: 1, PerPage: api.MaxPerPage})
	if err != nil {
		return nil, err
	}
	items := []T{}
	err = first.Each(ctx, func(item T) error {
		items = append(items, item)
		return nil
	})
	return items, err
}
