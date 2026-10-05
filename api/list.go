package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// List is a user's list of releases, masters, artists or labels. In a user's
// lists, Items is empty: List returns one list with its items.
//
// The fields follow what Discogs sent in October 2026, which differs from its
// docs: id rather than list_id, date_added and date_changed rather than
// created_ts and modified_ts, uri rather than url, and a user who owns it.
type List struct {
	ID          int        `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Public      bool       `json:"public"`
	DateAdded   string     `json:"date_added"`
	DateChanged string     `json:"date_changed"`
	User        UserRef    `json:"user"`
	Items       []ListItem `json:"items"`
	ImageURL    string     `json:"image_url"`
	URI         string     `json:"uri"`
	ResourceURL string     `json:"resource_url"`

	raw json.RawMessage
}

type list List

func (l *List) UnmarshalJSON(b []byte) error { return decodeKeep(b, (*list)(l), &l.raw) }
func (l List) MarshalJSON() ([]byte, error)  { return encodeKept(l.raw, list(l)) }

// ListItem is one entry in a list. Type is "release", "master", "artist" or
// "label", and says which resource ID refers to.
type ListItem struct {
	ID           int    `json:"id"`
	Type         string `json:"type"`
	DisplayTitle string `json:"display_title"`
	Comment      string `json:"comment"`
	ImageURL     string `json:"image_url"`
	URI          string `json:"uri"`
	ResourceURL  string `json:"resource_url"`
}

// UserLists returns one page of the lists of username. Private lists are
// included only when the client is authenticated as their owner.
func (c *Client) UserLists(ctx context.Context, username string, page Page) (*Paginated[List], error) {
	u, err := c.endpoint("users", username, "lists")
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	if err := page.apply(q); err != nil {
		return nil, err
	}
	u.RawQuery = q.Encode()
	return getPage[List](ctx, c, u, "lists")
}

// List returns the list with id and all its items. A private list needs a
// token for its owner.
func (c *Client) List(ctx context.Context, id int) (*List, error) {
	if id < 1 {
		return nil, fmt.Errorf("api: list id %d must be 1 or more", id)
	}
	u, err := c.endpoint("lists", strconv.Itoa(id))
	if err != nil {
		return nil, err
	}
	var l List
	if err := c.get(ctx, u, &l); err != nil {
		return nil, err
	}
	return &l, nil
}
