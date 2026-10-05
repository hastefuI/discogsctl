package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// Want is one release in a wantlist. ID is the release ID. Notes is set only
// when the client is authenticated as the wantlist owner. Rating is the
// owner's rating of the release, the one /releases/{id}/rating/{username}
// holds, not a property of the wantlist entry.
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

// AddWant adds the release with id to the wantlist of username, which must be
// the token holder, and returns the new entry. Discogs ignores notes sent with
// an add, so when notes is not empty AddWant sets them with a second request,
// as EditWant does.
func (c *Client) AddWant(ctx context.Context, username string, id int, notes string) (*Want, error) {
	u, err := c.wantURL(username, id)
	if err != nil {
		return nil, err
	}
	var w Want
	if err := c.do(ctx, http.MethodPut, u, nil, &w); err != nil {
		return nil, err
	}
	if notes == "" {
		return &w, nil
	}
	return c.EditWant(ctx, username, id, notes)
}

// ErrEmptyNotes is returned by EditWant for empty notes, which Discogs ignores
// rather than clearing the notes. Removing the release and adding it again
// clears them, and resets the date it was added.
var ErrEmptyNotes = errors.New("api: Discogs ignores empty notes, so an edit cannot clear them; remove the release and add it again, which resets its date added")

// EditWant sets the notes on the release with id in the wantlist of username,
// which must be the token holder, and returns the entry. Empty notes return
// ErrEmptyNotes without a request.
//
// The docs put notes in the query string, but tested live in October 2026
// Discogs read them only from a JSON body. They also list a rating, which is
// left out here: it is the user's rating of the release, which outlives the
// wantlist entry, and it did not change reliably through this endpoint.
func (c *Client) EditWant(ctx context.Context, username string, id int, notes string) (*Want, error) {
	if notes == "" {
		return nil, ErrEmptyNotes
	}
	u, err := c.wantURL(username, id)
	if err != nil {
		return nil, err
	}
	var w Want
	if err := c.do(ctx, http.MethodPost, u, map[string]string{"notes": notes}, &w); err != nil {
		return nil, err
	}
	return &w, nil
}

// RemoveWant removes the release with id from the wantlist of username, which
// must be the token holder. A rating the user gave the release stays, since
// it belongs to the release.
func (c *Client) RemoveWant(ctx context.Context, username string, id int) error {
	u, err := c.wantURL(username, id)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodDelete, u, nil, nil)
}

func (c *Client) wantURL(username string, id int) (*url.URL, error) {
	if id < 1 {
		return nil, fmt.Errorf("api: release id %d must be 1 or more", id)
	}
	return c.endpoint("users", username, "wants", strconv.Itoa(id))
}
