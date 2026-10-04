package api

import (
	"context"
	"encoding/json"
	"net/url"
)

// Want is one release in a wantlist. ID is the release ID. Notes is set only
// when the client is authenticated as the wantlist owner.
type Want struct {
	ID               int              `json:"id"`
	Rating           int              `json:"rating"`
	Notes            string           `json:"notes"`
	DateAdded        string           `json:"date_added"`
	BasicInformation BasicInformation `json:"basic_information"`
	ResourceURL      string           `json:"resource_url"`

	raw json.RawMessage
}

type want Want

func (w *Want) UnmarshalJSON(b []byte) error { return decodeKeep(b, (*want)(w), &w.raw) }
func (w Want) MarshalJSON() ([]byte, error)  { return encodeKept(w.raw, want(w)) }

// Wantlist returns one page of the wantlist of username.
func (c *Client) Wantlist(ctx context.Context, username string, page Page) (*Paginated[Want], error) {
	u, err := c.endpoint("users", username, "wants")
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	if err := page.apply(q); err != nil {
		return nil, err
	}
	u.RawQuery = q.Encode()
	return getPage[Want](ctx, c, u, "wants")
}
