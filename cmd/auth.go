package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"go.hasteful.org/discogsctl/api"
	"go.hasteful.org/discogsctl/output"
)

func newAuthCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate with the Discogs API and check the credentials in use",
	}
	cmd.AddCommand(newVerifyCmd(cfg), newExchangeCmd(cfg), newStatusCmd(cfg), newIdentityCmd(cfg), newWhoamiCmd(cfg))
	return cmd
}

// verifyTypes are the types auth verify takes. OAuth tokens come from auth
// exchange, which checks them itself.
var verifyTypes = []string{typePAT, typeConsumer}

// verifyResult is what auth verify prints: whether the credential was
// accepted, never the credential itself, which the user already has.
type verifyResult struct {
	Type     string `json:"type"`
	Verified bool   `json:"verified"`
	Username string `json:"username,omitzero"`
}

// exchanged is what auth exchange prints, once: the OAuth access token and
// secret and whose they are. Neither view is NAME=value shell, so it is never
// something to eval.
type exchanged struct {
	Username         string `json:"username"`
	OAuthToken       string `json:"oauth_token"`
	OAuthTokenSecret string `json:"oauth_token_secret"`
}

func newVerifyCmd(cfg *config) *cobra.Command {
	return &cobra.Command{
		Use:   "verify <type>",
		Short: "Check a personal access token or a consumer key and secret with Discogs",
		Long: `Read credentials of one type from stdin and check them with Discogs, without
storing or printing them. The type is one of:

  pat        A personal access token from
             ` + tokenURL + `, checked with
             GET /oauth/identity.
  consumer   An application's consumer key and secret, one per line, checked
             with a one result search, which needs authentication.

It prints only the result: a line of text, or with --output json
{"type", "verified", "username"}. A credential Discogs refuses is an error,
which exits 3. Prompts go to stderr, and there are none with --output json.

Nothing is stored. To use the credentials, set ` + envToken + `, or
` + envConsumerKey + ` and ` + envConsumerSecret + `. Input is not
hidden, so pipe a value in from a password manager to keep it off the screen.
verify ignores the credentials already configured, so it works while they are
broken.`,
		Example: `  discogsctl auth verify pat
  printf '%s\n%s\n' "$KEY" "$SECRET" | discogsctl auth verify consumer --output json`,
		ValidArgs: verifyTypes,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 || !slices.Contains(verifyTypes, args[0]) {
				return fmt.Errorf("auth verify takes one type: %s", strings.Join(verifyTypes, ", "))
			}
			return nil
		},
		// Only read the configuration: verify checks new credentials, so
		// configured ones that newClient would refuse must not stop it.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return cfg.prepare(cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			in := bufio.NewReader(cmd.InOrStdin())
			if args[0] == typePAT {
				return cfg.verifyPAT(cmd, in)
			}
			return cfg.verifyConsumer(cmd, in)
		},
	}
}

func newExchangeCmd(cfg *config) *cobra.Command {
	return &cobra.Command{
		Use:   "exchange",
		Short: "Run the OAuth flow and print the access token once",
		Long: `Run the OAuth 1.0a flow for the application in ` + envConsumerKey + `
and ` + envConsumerSecret + `, and print the access token and secret
Discogs issues for the user who approves it.

It writes a Discogs URL to stderr for the user to open and approve, then reads
the verification code Discogs shows them from stdin. If the application has a
callback URL, the address Discogs redirected to can be pasted in place of the
code. The access token is checked with GET /oauth/identity.

The token is printed once and not stored: a labelled block, or with
--output json {"username", "oauth_token", "oauth_token_secret"}. Neither is
shell to eval. Save it in a password manager straight away, then set
` + envOAuthToken + ` and ` + envOAuthTokenSecret + ` with the consumer key
and secret to act as that user. With --output json, the URL to open is written
to stderr as {"authorize_url": "..."} and there are no other prompts or
messages.`,
		Example: `  discogsctl auth exchange
  discogsctl auth exchange --output json | jq -r .oauth_token_secret`,
		Args: cobra.NoArgs,
		// Only read the configuration, as verify does; exchange uses just the
		// consumer key and secret.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return cfg.prepare(cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cfg.exchange(cmd, bufio.NewReader(cmd.InOrStdin()))
		},
	}
}

// verifyPAT reads a personal access token, checks whose it is, and prints
// only that.
func (cfg *config) verifyPAT(cmd *cobra.Command, in *bufio.Reader) error {
	token, err := cfg.prompt(cmd, in, "personal access token")
	if err != nil {
		return err
	}
	client, err := cfg.clientWith(token, api.OAuth{})
	if err != nil {
		return err
	}
	id, err := client.Identity(cmd.Context())
	if err != nil {
		return fmt.Errorf("checking the token: %w", err)
	}
	return cfg.printVerified(cmd, verifyResult{Type: typePAT, Verified: true, Username: id.Username},
		fmt.Sprintf("Verified: the token belongs to %s.\nTo use it, set %s.", id.Username, envToken))
}

// verifyConsumer reads a consumer key and secret and checks that Discogs
// accepts them. They act as no user, so the check is a search, which needs
// authentication of any kind.
func (cfg *config) verifyConsumer(cmd *cobra.Command, in *bufio.Reader) error {
	key, err := cfg.prompt(cmd, in, "consumer key")
	if err != nil {
		return err
	}
	secret, err := cfg.prompt(cmd, in, "consumer secret")
	if err != nil {
		return err
	}
	client, err := cfg.clientWith("", api.OAuth{ConsumerKey: key, ConsumerSecret: secret})
	if err != nil {
		return err
	}
	if _, err := client.Search(cmd.Context(), api.SearchQuery{Query: "discogs", Page: api.Page{Page: 1, PerPage: 1}}); err != nil {
		return fmt.Errorf("checking the consumer key and secret: %w", err)
	}
	return cfg.printVerified(cmd, verifyResult{Type: typeConsumer, Verified: true},
		fmt.Sprintf("Verified: Discogs accepted the consumer key and secret.\nTo use them, set %s and %s.", envConsumerKey, envConsumerSecret))
}

// printVerified prints the result of verify: text, or r in JSON mode.
func (cfg *config) printVerified(cmd *cobra.Command, r verifyResult, text string) error {
	if cfg.output == output.FormatJSON {
		return cfg.print(cmd, r)
	}
	_, err := fmt.Fprintln(cmd.OutOrStdout(), text)
	return err
}

// exchange runs the OAuth flow for the configured application, checks whose
// the access token is, and prints it once as JSON.
func (cfg *config) exchange(cmd *cobra.Command, in *bufio.Reader) error {
	consumer := api.OAuth{ConsumerKey: cfg.oauth.ConsumerKey, ConsumerSecret: cfg.oauth.ConsumerSecret}
	if consumer.ConsumerKey == "" || consumer.ConsumerSecret == "" {
		return fmt.Errorf("auth exchange needs %s and %s, from an application registered at %s", envConsumerKey, envConsumerSecret, tokenURL)
	}
	client, err := cfg.clientWith("", consumer)
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	request, err := client.OAuthRequestToken(ctx, "")
	if err != nil {
		return fmt.Errorf("getting a request token: %w", err)
	}
	// The user has to open the URL, so JSON mode writes it too, as one object
	// on stderr like a JSON error.
	if cfg.output == output.FormatJSON {
		if err := output.Write(cmd.ErrOrStderr(), output.FormatJSON, map[string]string{"authorize_url": request.AuthorizeURL()}); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(cmd.ErrOrStderr(), "Open this URL, approve the application, then paste the verification code:\n\n  %s\n\n", request.AuthorizeURL())
	}
	code, err := cfg.prompt(cmd, in, "verification code")
	if err != nil {
		return err
	}
	access, err := client.OAuthAccessToken(ctx, *request, verifier(code))
	if err != nil {
		return fmt.Errorf("getting an access token: %w", err)
	}

	user := consumer
	user.AccessToken, user.AccessSecret = access.Token, access.Secret
	if client, err = cfg.clientWith("", user); err != nil {
		return err
	}
	id, err := client.Identity(ctx)
	if err != nil {
		return fmt.Errorf("checking the access token: %w", err)
	}
	result := exchanged{Username: id.Username, OAuthToken: access.Token, OAuthTokenSecret: access.Secret}
	if cfg.output == output.FormatJSON {
		return cfg.print(cmd, result)
	}
	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 1, ' ', 0)
	fmt.Fprintf(tw, "Username:\t%s\nToken:\t%s\nToken secret:\t%s\n", result.Username, result.OAuthToken, result.OAuthTokenSecret)
	if err := tw.Flush(); err != nil {
		return err
	}
	cfg.note(cmd, "\nThe access token belongs to %s. It is shown once and not stored, so save it now.\nTo use it, set %s and %s.\n", id.Username, envOAuthToken, envOAuthTokenSecret)
	return nil
}

// note writes a message for a person to stderr, and nothing in JSON mode,
// where stdout is the only output.
func (cfg *config) note(cmd *cobra.Command, format string, args ...any) {
	if cfg.output != output.FormatJSON {
		fmt.Fprintf(cmd.ErrOrStderr(), format, args...)
	}
}

// prompt asks for name on stderr, except in JSON mode, and reads one line from
// in, trimmed. An empty answer is an error.
func (cfg *config) prompt(cmd *cobra.Command, in *bufio.Reader, name string) (string, error) {
	cfg.note(cmd, "%s%s: ", strings.ToUpper(name[:1]), name[1:])
	line, err := in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("reading the %s: %w", name, err)
	}
	if line = strings.TrimSpace(line); line == "" {
		return "", fmt.Errorf("no %s was entered", name)
	}
	return line, nil
}

// verifier returns the verification code the user pasted, or the
// oauth_verifier parameter when they pasted the callback address Discogs
// redirected them to.
func verifier(input string) string {
	input = strings.TrimSpace(input)
	if u, err := url.Parse(input); err == nil {
		if v := u.Query().Get("oauth_verifier"); v != "" {
			return v
		}
	}
	return input
}

// The states auth status reports for each method.
const (
	statusInUse         = "in use"
	statusConfigured    = "configured"
	statusIncomplete    = "incomplete"
	statusNotConfigured = "not configured"
)

// The types auth status names each method by: one word each, for scripts.
const (
	typePAT      = "pat"
	typeOAuth1   = "oauth1"
	typeConsumer = "consumer"
)

// authMethod is one way to authenticate and how it is configured. It never
// holds a credential. Priority is the order api.New picks a method in, 1
// first, when more than one is configured. Variables names the environment
// variables the method is read from.
type authMethod struct {
	Priority  int      `json:"priority"`
	Method    string   `json:"method"`
	Type      string   `json:"type"`
	Status    string   `json:"status"`
	Variables []string `json:"variables"`
}

// authStatus is what auth status prints. RateLimit is the documented budget
// for the method in use, not a figure from Discogs.
type authStatus struct {
	Methods    []authMethod `json:"methods"`
	ActsAsUser bool         `json:"acts_as_user"`
	RateLimit  int          `json:"rate_limit,omitzero"`
}

func newStatusCmd(cfg *config) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show which authentication methods are configured and which is in use",
		Long: `Show the three ways to authenticate, whether each is configured, and which
one requests use, with the environment variables each is read from. It reads
the environment only. No request is sent and no credential is printed, so a
wrong or revoked credential still shows as configured; run auth whoami to check
one with Discogs.

It exits 1 when the configuration is one every other command would refuse,
such as a personal token and an OAuth access token set together, or a key
without its secret.`,
		Example: `  discogsctl auth status
  discogsctl auth status --output json | jq -r '.methods[] | select(.status == "in use") | .type'`,
		Args: cobra.NoArgs,
		// Only read the configuration: building the client would refuse the
		// broken configurations this command exists to explain.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return cfg.prepare(cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			st, problem := cfg.authStatus()
			var err error
			if cfg.output == output.FormatJSON {
				err = cfg.print(cmd, st)
			} else {
				err = writeAuthStatus(cmd.OutOrStdout(), st, problem == nil)
			}
			if problem != nil {
				return problem
			}
			return err
		},
	}
}

// authStatus reports how each method is configured and which one requests
// use, and returns the error newClient gives for the configuration, if any.
// Then no method is in use, since every other command would fail.
func (cfg *config) authStatus() (authStatus, error) {
	o := cfg.oauth
	token := authMethod{Priority: 1, Method: "Personal access token", Type: typePAT, Status: statusNotConfigured, Variables: []string{envToken}}
	if cfg.personalToken() != "" {
		token.Status = statusConfigured
	}
	access := credentialPair(authMethod{Priority: 2, Method: "OAuth access token", Type: typeOAuth1,
		Variables: []string{envOAuthToken, envOAuthTokenSecret}}, o.AccessToken, o.AccessSecret)
	consumer := credentialPair(authMethod{Priority: 3, Method: "Consumer key and secret", Type: typeConsumer,
		Variables: []string{envConsumerKey, envConsumerSecret}}, o.ConsumerKey, o.ConsumerSecret)
	if access.Status == statusConfigured && consumer.Status != statusConfigured {
		access.Status = statusIncomplete
	}

	var st authStatus
	client, err := cfg.newClient()
	if err == nil {
		// The same precedence api.New applies. OAuth signs with the consumer
		// key and secret, so both are in use together.
		switch {
		case token.Status == statusConfigured:
			token.Status = statusInUse
		case access.Status == statusConfigured:
			access.Status, consumer.Status = statusInUse, statusInUse
		case consumer.Status == statusConfigured:
			consumer.Status = statusInUse
		}
		st.ActsAsUser = client.Authenticated()
		st.RateLimit = 25
		if consumer.Status == statusInUse || token.Status == statusInUse {
			st.RateLimit = 60
		}
	}
	// In order of priority: either user credential is used over the
	// consumer key and secret alone.
	st.Methods = []authMethod{token, access, consumer}
	return st, err
}

// credentialPair sets the status of m, a method made of two values, which is
// incomplete when only one is set.
func credentialPair(m authMethod, a, b string) authMethod {
	m.Status = statusConfigured
	switch {
	case a == "" && b == "":
		m.Status = statusNotConfigured
	case a == "" || b == "":
		m.Status = statusIncomplete
	}
	return m
}

// writeAuthStatus prints each method, its type, its status and its variables
// as a table and, when the configuration works, a line on what requests get.
func writeAuthStatus(w io.Writer, st authStatus, usable bool) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PRIORITY\tMETHOD\tTYPE\tSTATUS\tVARIABLES")
	for _, m := range st.Methods {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n", m.Priority, m.Method, m.Type, m.Status, strings.Join(m.Variables, ", "))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if !usable {
		return nil
	}
	switch {
	case st.ActsAsUser:
		_, err := fmt.Fprintf(w, "\nRequests act as a user, at up to %d a minute, with image URLs.\n", st.RateLimit)
		return err
	case st.RateLimit > 25:
		_, err := fmt.Fprintf(w, "\nRequests act as no user, at up to %d a minute, with image URLs.\n", st.RateLimit)
		return err
	default:
		_, err := fmt.Fprintf(w, "\nRequests are anonymous, at up to %d a minute, without image URLs.\n", st.RateLimit)
		return err
	}
}
