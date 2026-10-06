// Package cmd is the discogsctl command tree. Each resource is a file with a
// constructor that root.go registers.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"go.hasteful.org/discogsctl/api"
	"go.hasteful.org/discogsctl/dump"
	"go.hasteful.org/discogsctl/internal/cli"
	"go.hasteful.org/discogsctl/output"
)

const (
	envToken   = "DISCOGSCTL_TOKEN"
	projectURL = "https://hasteful.dev/discogsctl"
	tokenURL   = "https://www.discogs.com/settings/developers"

	// Credentials have no flags, since a flag shows up in ps and shell
	// history. They are read from the environment only.
	envConsumerKey      = "DISCOGSCTL_CONSUMER_KEY"
	envConsumerSecret   = "DISCOGSCTL_CONSUMER_SECRET"
	envOAuthToken       = "DISCOGSCTL_OAUTH_TOKEN"
	envOAuthTokenSecret = "DISCOGSCTL_OAUTH_TOKEN_SECRET"
)

// credentialsHint says how to authenticate, by either method.
var credentialsHint = fmt.Sprintf("set %s to a personal access token from %s, or use OAuth (see discogsctl auth exchange --help)", envToken, tokenURL)

// VersionInfo is stamped into the binary at link time.
type VersionInfo struct {
	Version string
	Commit  string
	Date    string
}

// userAgent is the one agent the CLI sends. There is deliberately no flag or
// environment variable to change it: Discogs answers a generic agent with an
// empty body, and wants a unique one so it can contact the app's owner
// rather than block it.
func userAgent(version string) string {
	return fmt.Sprintf("discogsctl/%s (+%s)", version, projectURL)
}

type config struct {
	oauth    api.OAuth
	output   string
	currency string
	verbose  bool

	// baseURL is the Discogs host and dumpURL the data dump host. Tests point
	// them at a fake server.
	baseURL string
	dumpURL string
	agent   string
	client  *api.Client
	logger  *slog.Logger
}

// Execute runs the command line and returns the exit code.
func Execute(vi VersionInfo) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root, cfg := newRootCmd(vi)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return cli.ExitOK
	}
	cli.PrintError(os.Stderr, cfg.output, err)
	code := cli.ExitCode(err)
	if code == cli.ExitUnauthorized && cfg.output != output.FormatJSON && (cfg.client == nil || !cfg.client.Authenticated()) {
		fmt.Fprintf(os.Stderr, "To authenticate, %s\n", credentialsHint)
	}
	return code
}

func newRootCmd(vi VersionInfo) (*cobra.Command, *config) {
	cfg := &config{baseURL: api.DefaultBaseURL, dumpURL: dump.DefaultBaseURL, agent: userAgent(vi.Version)}

	root := &cobra.Command{
		Use:   "discogsctl",
		Short: "A command line client for the Discogs API",
		Long: `discogsctl reads the Discogs database, collections and wantlists.

Authenticate in one of three ways, all sent in the Authorization header,
never in a URL:

  Personal access token: generate one at ` + tokenURL + `
  and set ` + envToken + `. It acts as your own account.

  Consumer key and secret: register an application at the same page and set
  ` + envConsumerKey + ` and ` + envConsumerSecret + `. Requests get
  60 a minute and image URLs without acting as any user, for public reads at
  volume.

  OAuth 1.0a: with the consumer key and secret set, run discogsctl auth
  exchange to get an access token for the user who approves it. Then also set
  ` + envOAuthToken + ` and ` + envOAuthTokenSecret + `.

discogsctl auth verify checks a token or a key and secret with Discogs before
you set them, and auth status shows which are set. Nothing is stored.

A personal token and an OAuth access token cannot both be set, and either one
is used over a consumer key and secret alone. Without any credentials,
requests are limited to 25 a minute, image URLs are left out, and auth whoami
and private collections are unavailable.

Output is text by default. --output json prints the Discogs body itself; a
listing prints the array of items. Errors go to stderr, and the exit status
is 3 for 401, 4 for 404, 5 for 429 and 1 for any other failure.`,
		Version:       vi.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return cfg.connect(cmd)
		},
	}
	root.SetVersionTemplate(fmt.Sprintf("discogsctl %s\ncommit: %s\nbuilt: %s\n", vi.Version, vi.Commit, vi.Date))

	pf := root.PersistentFlags()
	pf.StringVarP(&cfg.output, "output", "o", output.FormatText, "output format: "+strings.Join(output.ValidFormats(), " or "))
	pf.StringVar(&cfg.currency, "currency", "USD", "currency for marketplace prices: "+strings.Join(api.Currencies, ", "))
	pf.BoolVar(&cfg.verbose, "verbose", false, "log each request, the rate limit headers and pagination to stderr")

	root.AddCommand(
		newSearchCmd(cfg),
		newReleaseCmd(cfg),
		newMasterCmd(cfg),
		newArtistCmd(cfg),
		newLabelCmd(cfg),
		newCollectionCmd(cfg),
		newWantlistCmd(cfg),
		newMarketplaceCmd(cfg),
		newUserCmd(cfg),
		newListCmd(cfg),
		newDumpCmd(cfg),
		newExportCmd(cfg),
		newAuthCmd(cfg),
		newVersionCmd(vi),
	)
	return root, cfg
}

// connect validates the persistent flags and builds the client. It sends no
// request.
func (cfg *config) connect(cmd *cobra.Command) error {
	if err := cfg.prepare(cmd); err != nil {
		return err
	}
	client, err := cfg.newClient()
	if err != nil {
		return err
	}
	cfg.client = client
	return nil
}

// prepare validates --output, sets up logging and reads the OAuth credentials
// from the environment. It does not check the credentials, so auth status can
// run on a configuration that newClient would refuse.
func (cfg *config) prepare(cmd *cobra.Command) error {
	if !output.IsValidFormat(cfg.output) {
		return fmt.Errorf("invalid --output %q, want one of %s", cfg.output, strings.Join(output.ValidFormats(), ", "))
	}

	logger := slog.New(slog.DiscardHandler)
	if cfg.verbose {
		logger = slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	cfg.logger = logger

	cfg.oauth = api.OAuth{
		ConsumerKey:    strings.TrimSpace(os.Getenv(envConsumerKey)),
		ConsumerSecret: strings.TrimSpace(os.Getenv(envConsumerSecret)),
		AccessToken:    strings.TrimSpace(os.Getenv(envOAuthToken)),
		AccessSecret:   strings.TrimSpace(os.Getenv(envOAuthTokenSecret)),
	}
	return nil
}

// personalToken returns the personal access token from the environment.
func (cfg *config) personalToken() string {
	return strings.TrimSpace(os.Getenv(envToken))
}

// newClient checks the credentials from the environment and builds the
// client. It sends no request.
func (cfg *config) newClient() (*api.Client, error) {
	token := cfg.personalToken()
	if token != "" && cfg.oauth.AccessToken != "" {
		return nil, fmt.Errorf("both %s and %s are set: use one or the other", envToken, envOAuthToken)
	}
	return cfg.clientWith(token, cfg.oauth)
}

// clientWith builds a client with the given credentials instead of the
// configured ones, as auth verify and auth exchange do to check what the user
// entered. It sends no request.
func (cfg *config) clientWith(token string, oauth api.OAuth) (*api.Client, error) {
	return api.New(api.Options{
		BaseURL:   cfg.baseURL,
		UserAgent: cfg.agent,
		Token:     token,
		OAuth:     oauth,
		Currency:  cfg.currency,
		Logger:    cfg.logger,
	})
}

func (cfg *config) print(cmd *cobra.Command, v any) error {
	return output.Write(cmd.OutOrStdout(), cfg.output, v)
}

// username returns name, or the token holder's username when name is empty.
func (cfg *config) username(ctx context.Context, name string) (string, error) {
	if name != "" {
		return name, nil
	}
	if !cfg.client.Authenticated() {
		return "", errors.New("--username is required without a token")
	}
	id, err := cfg.client.Identity(ctx)
	if err != nil {
		return "", err
	}
	return id.Username, nil
}

// tokenHolder returns the username of the token holder, whose account a
// write changes. Without a token it fails before any request is sent.
func (cfg *config) tokenHolder(ctx context.Context) (string, error) {
	if !cfg.client.Authenticated() {
		return "", fmt.Errorf("this changes your account, so it needs a token: %s", credentialsHint)
	}
	return cfg.username(ctx, "")
}

func parseID(kind, arg string) (int, error) {
	id, err := strconv.Atoi(arg)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("invalid %s id %q: want a positive number", kind, arg)
	}
	return id, nil
}

// listRun returns a RunE that prints the items of a paginated listing,
// honouring --page, --per-page and --all.
func listRun[T any](cfg *config, pages *cli.PageFlags, fetch func(ctx context.Context, args []string, page api.Page) (*api.Paginated[T], error)) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		items, err := cli.Collect(cmd.Context(), pages, func(ctx context.Context, page api.Page) (*api.Paginated[T], error) {
			return fetch(ctx, args, page)
		})
		if err != nil {
			return err
		}
		return cfg.print(cmd, items)
	}
}

func newVersionCmd(vi VersionInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "discogsctl %s\ncommit: %s\nbuilt: %s\nuser agent: %s\n", vi.Version, vi.Commit, vi.Date, userAgent(vi.Version))
			return nil
		},
	}
}
