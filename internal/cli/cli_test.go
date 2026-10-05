package cli

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/spf13/pflag"
	"go.hasteful.org/discogsctl/api"
	"go.hasteful.org/discogsctl/dump"
)

func TestExitCode(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{nil, ExitOK},
		{&api.Error{StatusCode: 401}, ExitUnauthorized},
		{&api.Error{StatusCode: 404}, ExitNotFound},
		{&api.Error{StatusCode: 429}, ExitRateLimited},
		{fmt.Errorf("wrapped: %w", &api.Error{StatusCode: 404}), ExitNotFound},
		{&api.Error{StatusCode: 403}, ExitFailure},
		{&api.Error{StatusCode: 500}, ExitFailure},
		{errors.New("boom"), ExitFailure},
		{fmt.Errorf("dump: no dump with ID 2030: %w", dump.ErrNotFound), ExitNotFound},
	}
	for _, tt := range tests {
		if got := ExitCode(tt.err); got != tt.want {
			t.Errorf("ExitCode(%v) = %d, want %d", tt.err, got, tt.want)
		}
	}
}

func TestPrintErrorJSON(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{&api.Error{StatusCode: 404, Message: "Release not found."}, `{"status":404,"message":"Release not found."}` + "\n"},
		{errors.New("--username is required"), `{"message":"--username is required"}` + "\n"},
	}
	for _, tt := range tests {
		var buf bytes.Buffer
		PrintError(&buf, "json", tt.err)
		if buf.String() != tt.want {
			t.Errorf("PrintError(%v) = %q, want %q", tt.err, buf.String(), tt.want)
		}
	}
}

func TestPageFlags(t *testing.T) {
	tests := []struct {
		args    []string
		want    api.Page
		wantErr bool
	}{
		{nil, api.Page{Page: 1, PerPage: DefaultPerPage}, false},
		{[]string{"--page", "3", "--per-page", "10"}, api.Page{Page: 3, PerPage: 10}, false},
		{[]string{"--all"}, api.Page{Page: 1, PerPage: api.MaxPerPage}, false},
		{[]string{"--all", "--per-page", "20"}, api.Page{Page: 1, PerPage: 20}, false},
		{[]string{"--page", "0"}, api.Page{}, true},
		{[]string{"--per-page", "101"}, api.Page{}, true},
	}
	for _, tt := range tests {
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		f := BindPageFlags(fs)
		if err := fs.Parse(tt.args); err != nil {
			t.Fatal(err)
		}
		got, err := f.Page()
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("args %v: Page() = %+v, %v, want %+v, wantErr %v", tt.args, got, err, tt.want, tt.wantErr)
		}
	}
}
