package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// Identity is the user the client is authenticated as.
type Identity struct {
	ID           int    `json:"id"`
	Username     string `json:"username"`
	ConsumerName string `json:"consumer_name"`
	ResourceURL  string `json:"resource_url"`

	raw json.RawMessage
}

type identity Identity

func (i *Identity) UnmarshalJSON(b []byte) error { return decodeKeep(b, (*identity)(i), &i.raw) }
func (i Identity) MarshalJSON() ([]byte, error)  { return encodeKept(i.raw, identity(i)) }

// User is a Discogs user profile. Email is set only when the client is
// authenticated as that user. NumCollection and NumWantlist are nil when the
// collection or wantlist is private and the client is not its owner, because
// Discogs leaves the count out rather than sending zero. RatingAvg is the
// average of the ratings the user has given releases.
//
// Activated, MarketplaceSuspended and IsStaff are not in the Discogs docs but
// are in live profiles. Each is nil when a response leaves it out, so a
// missing field is not read as false. The docs list NumPending without saying
// what it counts.
type User struct {
	ID                   int     `json:"id"`
	Username             string  `json:"username"`
	Name                 string  `json:"name"`
	Email                string  `json:"email"`
	Location             string  `json:"location"`
	Profile              string  `json:"profile"`
	HomePage             string  `json:"home_page"`
	Registered           string  `json:"registered"`
	NumCollection        *int    `json:"num_collection"`
	NumWantlist          *int    `json:"num_wantlist"`
	NumForSale           int     `json:"num_for_sale"`
	NumLists             int     `json:"num_lists"`
	NumPending           int     `json:"num_pending"`
	Rank                 float64 `json:"rank"`
	RatingAvg            float64 `json:"rating_avg"`
	ReleasesContributed  int     `json:"releases_contributed"`
	ReleasesRated        int     `json:"releases_rated"`
	BuyerRating          float64 `json:"buyer_rating"`
	BuyerRatingStars     float64 `json:"buyer_rating_stars"`
	BuyerNumRatings      int     `json:"buyer_num_ratings"`
	SellerRating         float64 `json:"seller_rating"`
	SellerRatingStars    float64 `json:"seller_rating_stars"`
	SellerNumRatings     int     `json:"seller_num_ratings"`
	CurrAbbr             string  `json:"curr_abbr"`
	Activated            *bool   `json:"activated"`
	MarketplaceSuspended *bool   `json:"marketplace_suspended"`
	IsStaff              *bool   `json:"is_staff"`
	AvatarURL            string  `json:"avatar_url"`
	BannerURL            string  `json:"banner_url"`
	URI                  string  `json:"uri"`
	ResourceURL          string  `json:"resource_url"`
	InventoryURL         string  `json:"inventory_url"`
	CollectionFoldersURL string  `json:"collection_folders_url"`
	CollectionFieldsURL  string  `json:"collection_fields_url"`
	WantlistURL          string  `json:"wantlist_url"`

	raw json.RawMessage
}

type user User

func (u *User) UnmarshalJSON(b []byte) error { return decodeKeep(b, (*user)(u), &u.raw) }
func (u User) MarshalJSON() ([]byte, error)  { return encodeKept(u.raw, user(u)) }

// Identity returns the user the token belongs to. Without a token Discogs
// answers 401.
func (c *Client) Identity(ctx context.Context) (*Identity, error) {
	u, err := c.endpoint("oauth", "identity")
	if err != nil {
		return nil, err
	}
	var i Identity
	if err := c.get(ctx, u, &i); err != nil {
		return nil, err
	}
	return &i, nil
}

// User returns the profile of username.
func (c *Client) User(ctx context.Context, username string) (*User, error) {
	u, err := c.endpoint("users", username)
	if err != nil {
		return nil, err
	}
	var p User
	if err := c.get(ctx, u, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ProfileEdit changes fields of a user's profile. A nil field is left as it
// is, and an empty string clears it. Currency is the account's currency for
// marketplace prices, one of Currencies. The username cannot be changed
// through the API.
type ProfileEdit struct {
	Name     *string
	HomePage *string
	Location *string
	Profile  *string
	Currency *string
}

// EditProfile changes the profile of username, which must be the token
// holder, and returns the profile as it now is. It sends only the fields e
// sets, as a JSON body; tested live in October 2026, a field left out kept
// its value and an empty one was cleared.
func (c *Client) EditProfile(ctx context.Context, username string, e ProfileEdit) (*User, error) {
	body := map[string]string{}
	for name, value := range map[string]*string{
		"name":      e.Name,
		"home_page": e.HomePage,
		"location":  e.Location,
		"profile":   e.Profile,
		"curr_abbr": e.Currency,
	} {
		if value != nil {
			body[name] = *value
		}
	}
	if len(body) == 0 {
		return nil, errors.New("api: a profile edit must change at least one field")
	}
	if e.Currency != nil && !slices.Contains(Currencies, *e.Currency) {
		return nil, fmt.Errorf("api: currency %q must be one of %s", *e.Currency, strings.Join(Currencies, ", "))
	}
	u, err := c.endpoint("users", username)
	if err != nil {
		return nil, err
	}
	var p User
	if err := c.do(ctx, http.MethodPost, u, body, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ContributionSorts are the values Discogs accepts for ContributionQuery.Sort.
var ContributionSorts = []string{"label", "artist", "title", "catno", "format", "rating", "year", "added"}

// ContributionQuery sorts a user's contributions. Both fields are optional,
// and Discogs lists the most recently added first without them. SortOrder is
// "asc" or "desc".
type ContributionQuery struct {
	Sort      string
	SortOrder string

	Page
}

// Contributions returns one page of the releases username has contributed
// to the database. They are public, so no token is needed.
func (c *Client) Contributions(ctx context.Context, username string, q ContributionQuery) (*Paginated[Release], error) {
	u, err := c.endpoint("users", username, "contributions")
	if err != nil {
		return nil, err
	}
	v := url.Values{}
	if err := sortValues(v, "contribution", q.Sort, q.SortOrder, ContributionSorts); err != nil {
		return nil, err
	}
	if err := q.Page.apply(v); err != nil {
		return nil, err
	}
	u.RawQuery = v.Encode()
	return getPage[Release](ctx, c, u, "contributions")
}

// Submissions are a user's edits to the database, by kind. Each entry is the
// artist, label or release as Discogs sends it.
type Submissions struct {
	Artists  []Artist  `json:"artists"`
	Labels   []Label   `json:"labels"`
	Releases []Release `json:"releases"`
}

// SubmissionsPage is one page of a user's submissions. Discogs pages the three
// kinds together, so a page of 50 holds 50 artists, labels and releases
// combined, and Pagination counts them all.
type SubmissionsPage struct {
	Pagination  Pagination
	Submissions Submissions

	client *Client
}

// Submissions returns the first page of the edits username has made to
// artists, labels and releases. They are public, so no token is needed.
func (c *Client) Submissions(ctx context.Context, username string, page Page) (*SubmissionsPage, error) {
	u, err := c.endpoint("users", username, "submissions")
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	if err := page.apply(q); err != nil {
		return nil, err
	}
	u.RawQuery = q.Encode()
	return c.submissionsPage(ctx, u)
}

// Next fetches the page after p, following pagination.urls.next. It returns
// nil and no error when p is the last page.
func (p *SubmissionsPage) Next(ctx context.Context) (*SubmissionsPage, error) {
	if p.Pagination.URLs.Next == "" {
		return nil, nil
	}
	u, err := p.client.follow(p.Pagination.URLs.Next)
	if err != nil {
		return nil, err
	}
	return p.client.submissionsPage(ctx, u)
}

// All returns the submissions on p and on every page after it, each kind
// joined across pages.
func (p *SubmissionsPage) All(ctx context.Context) (Submissions, error) {
	all := Submissions{Artists: []Artist{}, Labels: []Label{}, Releases: []Release{}}
	for page := p; page != nil; {
		all.Artists = append(all.Artists, page.Submissions.Artists...)
		all.Labels = append(all.Labels, page.Submissions.Labels...)
		all.Releases = append(all.Releases, page.Submissions.Releases...)
		next, err := page.Next(ctx)
		if err != nil {
			return all, err
		}
		page = next
	}
	return all, nil
}

func (c *Client) submissionsPage(ctx context.Context, u *url.URL) (*SubmissionsPage, error) {
	var body struct {
		Pagination  Pagination  `json:"pagination"`
		Submissions Submissions `json:"submissions"`
	}
	if err := c.get(ctx, u, &body); err != nil {
		return nil, err
	}
	s := body.Submissions
	if s.Artists == nil {
		s.Artists = []Artist{}
	}
	if s.Labels == nil {
		s.Labels = []Label{}
	}
	if s.Releases == nil {
		s.Releases = []Release{}
	}
	return &SubmissionsPage{Pagination: body.Pagination, Submissions: s, client: c}, nil
}
