// Package cli holds the pieces of the command line that are not commands:
// exit codes, error output and the shared pagination flags.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/pflag"
	"go.hasteful.org/discogsctl/api"
	"go.hasteful.org/discogsctl/dump"
	"go.hasteful.org/discogsctl/output"
)

// Exit codes are the contract with scripts.
const (
	ExitOK           = 0
	ExitFailure      = 1
	ExitUnauthorized = 3
	ExitNotFound     = 4
	ExitRateLimited  = 5
)

// ErrNotFound is wrapped by a command's error when what it was asked for does
// not exist without Discogs answering 404, such as a release that is not in a
// collection, so it exits 4 like a 404.
var ErrNotFound = errors.New("not found")

// ExitCode maps err to an exit code: 401 is 3, 404 or anything else that does
// not exist is 4, 429 is 5, and any other failure is 1.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, dump.ErrNotFound) {
		return ExitNotFound
	}
	if apiErr, ok := errors.AsType[*api.Error](err); ok {
		switch apiErr.StatusCode {
		case http.StatusUnauthorized:
			return ExitUnauthorized
		case http.StatusNotFound:
			return ExitNotFound
		case http.StatusTooManyRequests:
			return ExitRateLimited
		}
	}
	return ExitFailure
}

// PrintError writes err to w, which is standard error. In JSON mode it is one
// object, {"status":404,"message":"Release not found."}, with status left out
// when the failure was not an HTTP response.
func PrintError(w io.Writer, format string, err error) {
	if format != output.FormatJSON {
		fmt.Fprintf(w, "Error: %v\n", err)
		return
	}
	payload := struct {
		Status  int    `json:"status,omitzero"`
		Message string `json:"message"`
	}{Message: err.Error()}
	if apiErr, ok := errors.AsType[*api.Error](err); ok {
		payload.Status, payload.Message = apiErr.StatusCode, apiErr.Message
	}
	b, _ := json.Marshal(payload)
	fmt.Fprintf(w, "%s\n", b)
}

// DefaultPerPage is the page size unless --per-page says otherwise. It
// matches the Discogs default.
const DefaultPerPage = 50

// PageFlags are --page, --per-page and --all, shared by every listing.
type PageFlags struct {
	page    int
	perPage int
	all     bool
	flags   *pflag.FlagSet
}

// BindPageFlags adds the pagination flags to fs.
func BindPageFlags(fs *pflag.FlagSet) *PageFlags {
	f := &PageFlags{flags: fs}
	fs.IntVar(&f.page, "page", 1, "page to fetch")
	fs.IntVar(&f.perPage, "per-page", DefaultPerPage, fmt.Sprintf("items per page, at most %d", api.MaxPerPage))
	fs.BoolVar(&f.all, "all", false, fmt.Sprintf("fetch every page from --page on, %d per page unless --per-page is set", api.MaxPerPage))
	return f
}

// Page returns the page to request first. With --all and no --per-page it
// asks for the largest page, so a walk spends fewer requests of the rate
// limit.
func (f *PageFlags) Page() (api.Page, error) {
	if f.page < 1 {
		return api.Page{}, fmt.Errorf("--page must be 1 or more, got %d", f.page)
	}
	if f.perPage < 1 || f.perPage > api.MaxPerPage {
		return api.Page{}, fmt.Errorf("--per-page must be between 1 and %d, got %d", api.MaxPerPage, f.perPage)
	}
	perPage := f.perPage
	if f.all && !f.flags.Changed("per-page") {
		perPage = api.MaxPerPage
	}
	return api.Page{Page: f.page, PerPage: perPage}, nil
}

// Collect fetches the page the flags select, and with --all every page after
// it, and returns their items in order.
func Collect[T any](ctx context.Context, f *PageFlags, fetch func(context.Context, api.Page) (*api.Paginated[T], error)) ([]T, error) {
	page, err := f.Page()
	if err != nil {
		return nil, err
	}
	first, err := fetch(ctx, page)
	if err != nil {
		return nil, err
	}
	if !f.all {
		return first.Items, nil
	}
	items := []T{}
	err = first.Each(ctx, func(item T) error {
		items = append(items, item)
		return nil
	})
	return items, err
}
