package api

import (
	"context"
	"encoding/json"
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
type User struct {
	ID                  int     `json:"id"`
	Username            string  `json:"username"`
	Name                string  `json:"name"`
	Email               string  `json:"email"`
	Location            string  `json:"location"`
	Profile             string  `json:"profile"`
	HomePage            string  `json:"home_page"`
	Registered          string  `json:"registered"`
	NumCollection       *int    `json:"num_collection"`
	NumWantlist         *int    `json:"num_wantlist"`
	NumForSale          int     `json:"num_for_sale"`
	NumLists            int     `json:"num_lists"`
	Rank                float64 `json:"rank"`
	RatingAvg           float64 `json:"rating_avg"`
	ReleasesContributed int     `json:"releases_contributed"`
	ReleasesRated       int     `json:"releases_rated"`
	BuyerRating         float64 `json:"buyer_rating"`
	BuyerNumRatings     int     `json:"buyer_num_ratings"`
	SellerRating        float64 `json:"seller_rating"`
	SellerNumRatings    int     `json:"seller_num_ratings"`
	CurrAbbr            string  `json:"curr_abbr"`
	URI                 string  `json:"uri"`
	ResourceURL         string  `json:"resource_url"`

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
