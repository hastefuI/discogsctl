package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// ArtistCredit is an artist as credited on a release, master or track. Join
// is the text Discogs puts between this artist and the next, such as "&".
type ArtistCredit struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	ANV         string `json:"anv"`
	Join        string `json:"join"`
	Role        string `json:"role"`
	Tracks      string `json:"tracks"`
	ResourceURL string `json:"resource_url"`
}

// LabelCredit is a label or company as credited on a release.
type LabelCredit struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	Catno          string `json:"catno"`
	EntityType     string `json:"entity_type"`
	EntityTypeName string `json:"entity_type_name"`
	ResourceURL    string `json:"resource_url"`
}

// Format is one physical or digital format of a release.
type Format struct {
	Name         string   `json:"name"`
	Qty          string   `json:"qty"`
	Text         string   `json:"text"`
	Descriptions []string `json:"descriptions"`
}

// Track is one entry on a tracklist. Type is "track", "heading" or "index".
type Track struct {
	Position     string         `json:"position"`
	Type         string         `json:"type_"`
	Title        string         `json:"title"`
	Duration     string         `json:"duration"`
	ExtraArtists []ArtistCredit `json:"extraartists"`
}

// Rating is an average rating and the number of votes behind it.
type Rating struct {
	Average float64 `json:"average"`
	Count   int     `json:"count"`
}

// Community is the community data on a release.
type Community struct {
	Have   int    `json:"have"`
	Want   int    `json:"want"`
	Rating Rating `json:"rating"`
	Status string `json:"status"`
}

// Release is a particular physical or digital object released by one or more
// artists. LowestPrice is in the client's currency and nil when no copy is for
// sale.
type Release struct {
	ID          int            `json:"id"`
	Title       string         `json:"title"`
	Artists     []ArtistCredit `json:"artists"`
	Year        int            `json:"year"`
	Released    string         `json:"released"`
	Country     string         `json:"country"`
	Labels      []LabelCredit  `json:"labels"`
	Formats     []Format       `json:"formats"`
	Genres      []string       `json:"genres"`
	Styles      []string       `json:"styles"`
	Tracklist   []Track        `json:"tracklist"`
	MasterID    int            `json:"master_id"`
	Status      string         `json:"status"`
	DataQuality string         `json:"data_quality"`
	Community   Community      `json:"community"`
	NumForSale  int            `json:"num_for_sale"`
	LowestPrice *float64       `json:"lowest_price"`
	Notes       string         `json:"notes"`
	DateAdded   string         `json:"date_added"`
	DateChanged string         `json:"date_changed"`
	URI         string         `json:"uri"`
	ResourceURL string         `json:"resource_url"`

	raw json.RawMessage
}

type release Release

func (r *Release) UnmarshalJSON(b []byte) error { return decodeKeep(b, (*release)(r), &r.raw) }
func (r Release) MarshalJSON() ([]byte, error)  { return encodeKept(r.raw, release(r)) }

// ReleaseRating is the community rating of a release.
type ReleaseRating struct {
	ReleaseID int    `json:"release_id"`
	Rating    Rating `json:"rating"`

	raw json.RawMessage
}

type releaseRating ReleaseRating

func (r *ReleaseRating) UnmarshalJSON(b []byte) error {
	return decodeKeep(b, (*releaseRating)(r), &r.raw)
}
func (r ReleaseRating) MarshalJSON() ([]byte, error) { return encodeKept(r.raw, releaseRating(r)) }

// Master is a set of similar releases, with a main release that is often the
// earliest.
type Master struct {
	ID          int            `json:"id"`
	Title       string         `json:"title"`
	Artists     []ArtistCredit `json:"artists"`
	Year        int            `json:"year"`
	MainRelease int            `json:"main_release"`
	Genres      []string       `json:"genres"`
	Styles      []string       `json:"styles"`
	Tracklist   []Track        `json:"tracklist"`
	NumForSale  int            `json:"num_for_sale"`
	LowestPrice *float64       `json:"lowest_price"`
	DataQuality string         `json:"data_quality"`
	URI         string         `json:"uri"`
	ResourceURL string         `json:"resource_url"`

	raw json.RawMessage
}

type master Master

func (m *Master) UnmarshalJSON(b []byte) error { return decodeKeep(b, (*master)(m), &m.raw) }
func (m Master) MarshalJSON() ([]byte, error)  { return encodeKept(m.raw, master(m)) }

// MasterVersion is one release listed under a master.
type MasterVersion struct {
	ID           int      `json:"id"`
	Title        string   `json:"title"`
	Label        string   `json:"label"`
	Catno        string   `json:"catno"`
	Country      string   `json:"country"`
	Released     string   `json:"released"`
	Format       string   `json:"format"`
	MajorFormats []string `json:"major_formats"`
	Status       string   `json:"status"`
	ResourceURL  string   `json:"resource_url"`

	raw json.RawMessage
}

type masterVersion MasterVersion

func (v *MasterVersion) UnmarshalJSON(b []byte) error {
	return decodeKeep(b, (*masterVersion)(v), &v.raw)
}
func (v MasterVersion) MarshalJSON() ([]byte, error) { return encodeKept(v.raw, masterVersion(v)) }

// ArtistRef names another artist, such as a member of a group.
type ArtistRef struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Active      bool   `json:"active"`
	ResourceURL string `json:"resource_url"`
}

// Artist is a person or group that contributed to a release.
type Artist struct {
	ID             int         `json:"id"`
	Name           string      `json:"name"`
	RealName       string      `json:"realname"`
	Profile        string      `json:"profile"`
	NameVariations []string    `json:"namevariations"`
	URLs           []string    `json:"urls"`
	Members        []ArtistRef `json:"members"`
	Groups         []ArtistRef `json:"groups"`
	DataQuality    string      `json:"data_quality"`
	URI            string      `json:"uri"`
	ResourceURL    string      `json:"resource_url"`

	raw json.RawMessage
}

type artist Artist

func (a *Artist) UnmarshalJSON(b []byte) error { return decodeKeep(b, (*artist)(a), &a.raw) }
func (a Artist) MarshalJSON() ([]byte, error)  { return encodeKept(a.raw, artist(a)) }

// ArtistRelease is a release or master in an artist's discography. Type is
// "release" or "master", and MainRelease is set for a master.
type ArtistRelease struct {
	ID          int    `json:"id"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Role        string `json:"role"`
	Year        int    `json:"year"`
	Format      string `json:"format"`
	Label       string `json:"label"`
	Status      string `json:"status"`
	MainRelease int    `json:"main_release"`
	ResourceURL string `json:"resource_url"`

	raw json.RawMessage
}

type artistRelease ArtistRelease

func (r *ArtistRelease) UnmarshalJSON(b []byte) error {
	return decodeKeep(b, (*artistRelease)(r), &r.raw)
}
func (r ArtistRelease) MarshalJSON() ([]byte, error) { return encodeKept(r.raw, artistRelease(r)) }

// LabelRef names another label, such as a parent or sublabel.
type LabelRef struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	ResourceURL string `json:"resource_url"`
}

// Label is a label, company, studio or other entity credited on releases.
type Label struct {
	ID          int        `json:"id"`
	Name        string     `json:"name"`
	Profile     string     `json:"profile"`
	ContactInfo string     `json:"contact_info"`
	ParentLabel *LabelRef  `json:"parent_label"`
	Sublabels   []LabelRef `json:"sublabels"`
	URLs        []string   `json:"urls"`
	DataQuality string     `json:"data_quality"`
	URI         string     `json:"uri"`
	ResourceURL string     `json:"resource_url"`

	raw json.RawMessage
}

type label Label

func (l *Label) UnmarshalJSON(b []byte) error { return decodeKeep(b, (*label)(l), &l.raw) }
func (l Label) MarshalJSON() ([]byte, error)  { return encodeKept(l.raw, label(l)) }

// LabelRelease is a release in a label's catalogue.
type LabelRelease struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Catno       string `json:"catno"`
	Format      string `json:"format"`
	Year        int    `json:"year"`
	Status      string `json:"status"`
	ResourceURL string `json:"resource_url"`

	raw json.RawMessage
}

type labelRelease LabelRelease

func (r *LabelRelease) UnmarshalJSON(b []byte) error {
	return decodeKeep(b, (*labelRelease)(r), &r.raw)
}
func (r LabelRelease) MarshalJSON() ([]byte, error) { return encodeKept(r.raw, labelRelease(r)) }

// Release returns the release with id, with marketplace data in the client's
// currency.
func (c *Client) Release(ctx context.Context, id int) (*Release, error) {
	u, err := c.resource("releases", id)
	if err != nil {
		return nil, err
	}
	if c.currency != "" {
		u.RawQuery = url.Values{"curr_abbr": {c.currency}}.Encode()
	}
	var r Release
	if err := c.get(ctx, u, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// ReleaseRating returns the community rating of the release with id.
func (c *Client) ReleaseRating(ctx context.Context, id int) (*ReleaseRating, error) {
	u, err := c.resource("releases", id, "rating")
	if err != nil {
		return nil, err
	}
	var r ReleaseRating
	if err := c.get(ctx, u, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// UserRating is one user's rating of a release, 1 to 5.
type UserRating struct {
	ReleaseID int    `json:"release_id"`
	Username  string `json:"username"`
	Rating    int    `json:"rating"`

	raw json.RawMessage
}

type userRating UserRating

func (r *UserRating) UnmarshalJSON(b []byte) error { return decodeKeep(b, (*userRating)(r), &r.raw) }
func (r UserRating) MarshalJSON() ([]byte, error)  { return encodeKept(r.raw, userRating(r)) }

// RateRelease sets the rating username, which must be the token holder, gives
// the release with id, from 1 to 5, and returns it. It is the rating the
// user's wantlist shows for the release.
//
// The docs give no format for the rating. Tested live in October 2026,
// Discogs accepted it only as a string in a JSON body, {"rating": "3"}, and
// answered a JSON number, a query parameter or a form body with 422.
func (c *Client) RateRelease(ctx context.Context, id int, username string, rating int) (*UserRating, error) {
	if rating < 1 || rating > 5 {
		return nil, fmt.Errorf("api: rating %d must be between 1 and 5", rating)
	}
	u, err := c.resource("releases", id, "rating", username)
	if err != nil {
		return nil, err
	}
	var r UserRating
	if err := c.do(ctx, http.MethodPut, u, map[string]string{"rating": strconv.Itoa(rating)}, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// UnrateRelease removes the rating username, which must be the token holder,
// gave the release with id. Removing a rating that is not there succeeds.
func (c *Client) UnrateRelease(ctx context.Context, id int, username string) error {
	u, err := c.resource("releases", id, "rating", username)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodDelete, u, nil, nil)
}

// Master returns the master release with id.
func (c *Client) Master(ctx context.Context, id int) (*Master, error) {
	u, err := c.resource("masters", id)
	if err != nil {
		return nil, err
	}
	var m Master
	if err := c.get(ctx, u, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// MasterVersions returns one page of the releases under the master with id.
func (c *Client) MasterVersions(ctx context.Context, id int, page Page) (*Paginated[MasterVersion], error) {
	return listResource[MasterVersion](ctx, c, page, "versions", "masters", id, "versions")
}

// Artist returns the artist with id.
func (c *Client) Artist(ctx context.Context, id int) (*Artist, error) {
	u, err := c.resource("artists", id)
	if err != nil {
		return nil, err
	}
	var a Artist
	if err := c.get(ctx, u, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// ArtistReleases returns one page of the releases and masters of the artist
// with id.
func (c *Client) ArtistReleases(ctx context.Context, id int, page Page) (*Paginated[ArtistRelease], error) {
	return listResource[ArtistRelease](ctx, c, page, "releases", "artists", id, "releases")
}

// Label returns the label with id.
func (c *Client) Label(ctx context.Context, id int) (*Label, error) {
	u, err := c.resource("labels", id)
	if err != nil {
		return nil, err
	}
	var l Label
	if err := c.get(ctx, u, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// LabelReleases returns one page of the releases on the label with id.
func (c *Client) LabelReleases(ctx context.Context, id int, page Page) (*Paginated[LabelRelease], error) {
	return listResource[LabelRelease](ctx, c, page, "releases", "labels", id, "releases")
}

// resource returns the URL for kind/id followed by any further segments, and
// rejects an id below 1.
func (c *Client) resource(kind string, id int, rest ...string) (*url.URL, error) {
	if id < 1 {
		return nil, fmt.Errorf("api: %s id %d must be 1 or more", kind, id)
	}
	return c.endpoint(append([]string{kind, strconv.Itoa(id)}, rest...)...)
}

// listResource fetches one page of the listing at kind/id/sub, whose items are
// under key.
func listResource[T any](ctx context.Context, c *Client, page Page, key, kind string, id int, sub string) (*Paginated[T], error) {
	u, err := c.resource(kind, id, sub)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	if err := page.apply(q); err != nil {
		return nil, err
	}
	u.RawQuery = q.Encode()
	return getPage[T](ctx, c, u, key)
}
