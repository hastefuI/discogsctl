// Package api is a client for the Discogs API v2 at https://api.discogs.com.
//
// Create a Client with New. Discogs requires a User-Agent that names the
// calling application, so Options.UserAgent is mandatory and New rejects an
// empty or generic one:
//
//	client, err := api.New(api.Options{
//		UserAgent: "myapp/0.1 (+https://example.com)",
//		Token:     os.Getenv("DISCOGSCTL_TOKEN"),
//	})
//
// A personal access token is sent in the Authorization header, never in the
// query string, so it does not appear in URLs, logs or errors.
//
// An application's consumer key and secret, in Options.OAuth with no access
// token, are sent as "Discogs key=..., secret=...". Discogs gives them the
// same rate limit and image URLs as a token, without acting as any user, which
// suits public reads at volume. Client.Authenticated reports false for them.
//
// An application that acts on behalf of other users adds an OAuth 1.0a access
// token to Options.OAuth, in place of Token. With only the consumer key and
// secret, Client.OAuthRequestToken and Client.OAuthAccessToken run the
// authorization flow, and the access token they return goes back into
// Options.OAuth:
//
//	request, err := client.OAuthRequestToken(ctx, "")
//	// The user approves request.AuthorizeURL() and gets a verification code.
//	access, err := client.OAuthAccessToken(ctx, *request, code)
//
// Requests are signed with PLAINTEXT, as the Discogs docs describe, in the
// Authorization header.
//
// Every request goes through one transport that sets the User-Agent, the
// credentials and the Accept header, and waits on a shared rate limiter. The
// limiter starts at 25 requests a minute without credentials and 60 with any,
// a consumer key and secret alone included, then follows the
// X-Discogs-Ratelimit headers on each response. It keeps one request in
// reserve rather than running the window down to zero.
// Client.RateLimit returns those headers as Discogs last sent them.
//
// A response with a status of 400 or above is returned as an *Error.
//
// Values decoded from Discogs keep the body they were decoded from. Encoding
// one with encoding/json returns that body as Discogs sent it, including
// fields the Go type does not name.
package api
