// Package useragent checks that a User-Agent names the calling application.
// Discogs answers a missing or generic agent with an empty body, on the API
// and on the data dumps alike, so every client in this module checks it the
// same way.
package useragent

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var productVersion = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._+-]*$`)

// genericAgents are products that name an HTTP library or a browser rather
// than an application. Discogs lists curl and browser strings as bad agents.
var genericAgents = []string{
	"curl", "libcurl", "wget", "httpie",
	"mozilla", "applewebkit", "chrome", "safari", "gecko", "firefox",
	"go-http-client", "python-requests", "python-urllib", "okhttp", "java", "axios", "node-fetch", "postmanruntime",
}

// Check returns an error if ua is empty, does not start with product/version,
// or names an HTTP library or a browser.
func Check(ua string) error {
	fields := strings.Fields(ua)
	if len(fields) == 0 {
		return errors.New("UserAgent is required; Discogs identifies the calling application by it")
	}
	if !productVersion.MatchString(fields[0]) {
		return fmt.Errorf("UserAgent %q must start with product/version, such as myapp/1.0", ua)
	}
	for _, f := range fields {
		product, _, _ := strings.Cut(strings.ToLower(strings.Trim(f, "()+;,")), "/")
		if slices.Contains(genericAgents, product) {
			return fmt.Errorf("UserAgent %q names an HTTP library or browser, not the application", ua)
		}
	}
	return nil
}
