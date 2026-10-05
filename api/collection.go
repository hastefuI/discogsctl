package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// AllFolder is the folder ID that holds every release in a collection.
// Discogs serves it without authentication when the collection is public.
// Every other folder needs authentication as the owner.
const AllFolder = 0

// UncategorizedFolder is the folder ID of "Uncategorized", where a release is
// added unless another folder is chosen.
const UncategorizedFolder = 1

// Folder is a folder in a user's collection.
type Folder struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Count       int    `json:"count"`
	ResourceURL string `json:"resource_url"`

	raw json.RawMessage
}

type folder Folder

func (f *Folder) UnmarshalJSON(b []byte) error { return decodeKeep(b, (*folder)(f), &f.raw) }
func (f Folder) MarshalJSON() ([]byte, error)  { return encodeKept(f.raw, folder(f)) }

// BasicInformation is the summary of a release that collection and wantlist
// entries carry.
type BasicInformation struct {
	ID          int            `json:"id"`
	MasterID    int            `json:"master_id"`
	Title       string         `json:"title"`
	Year        int            `json:"year"`
	Artists     []ArtistCredit `json:"artists"`
	Labels      []LabelCredit  `json:"labels"`
	Formats     []Format       `json:"formats"`
	Genres      []string       `json:"genres"`
	Styles      []string       `json:"styles"`
	Thumb       string         `json:"thumb"`
	CoverImage  string         `json:"cover_image"`
	ResourceURL string         `json:"resource_url"`
}

// Note is the value of one custom notes field on a collection item.
type Note struct {
	FieldID int    `json:"field_id"`
	Value   string `json:"value"`
}

// CollectionItem is one instance of a release in a collection folder. ID is
// the release ID. A release owned twice has two items with different
// InstanceIDs.
type CollectionItem struct {
	ID               int              `json:"id"`
	InstanceID       int              `json:"instance_id"`
	FolderID         int              `json:"folder_id"`
	Rating           int              `json:"rating"`
	DateAdded        string           `json:"date_added"`
	BasicInformation BasicInformation `json:"basic_information"`
	Notes            []Note           `json:"notes"`

	raw json.RawMessage
}

type collectionItem CollectionItem

func (i *CollectionItem) UnmarshalJSON(b []byte) error {
	return decodeKeep(b, (*collectionItem)(i), &i.raw)
}
func (i CollectionItem) MarshalJSON() ([]byte, error) { return encodeKept(i.raw, collectionItem(i)) }

// CollectionValue is the estimated value of a collection, formatted by Discogs
// in the owner's currency, such as "$100.25".
type CollectionValue struct {
	Minimum string `json:"minimum"`
	Median  string `json:"median"`
	Maximum string `json:"maximum"`

	raw json.RawMessage
}

type collectionValue CollectionValue

func (v *CollectionValue) UnmarshalJSON(b []byte) error {
	return decodeKeep(b, (*collectionValue)(v), &v.raw)
}
func (v CollectionValue) MarshalJSON() ([]byte, error) { return encodeKept(v.raw, collectionValue(v)) }

// CollectionFolders returns the folders in the collection of username.
func (c *Client) CollectionFolders(ctx context.Context, username string) ([]Folder, error) {
	u, err := c.endpoint("users", username, "collection", "folders")
	if err != nil {
		return nil, err
	}
	var body struct {
		Folders []Folder `json:"folders"`
	}
	if err := c.get(ctx, u, &body); err != nil {
		return nil, err
	}
	if body.Folders == nil {
		body.Folders = []Folder{}
	}
	return body.Folders, nil
}

// CollectionItems returns one page of the items in folder of the collection of
// username. Use AllFolder for every item.
func (c *Client) CollectionItems(ctx context.Context, username string, folder int, page Page) (*Paginated[CollectionItem], error) {
	if folder < 0 {
		return nil, fmt.Errorf("api: folder id %d must be 0 or more", folder)
	}
	u, err := c.endpoint("users", username, "collection", "folders", strconv.Itoa(folder), "releases")
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	if err := page.apply(q); err != nil {
		return nil, err
	}
	u.RawQuery = q.Encode()
	return getPage[CollectionItem](ctx, c, u, "releases")
}

// CollectionValue returns the estimated value of the collection of username.
// Discogs requires authentication as the owner.
func (c *Client) CollectionValue(ctx context.Context, username string) (*CollectionValue, error) {
	u, err := c.endpoint("users", username, "collection", "value")
	if err != nil {
		return nil, err
	}
	var v CollectionValue
	if err := c.get(ctx, u, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// CollectionInstance is a copy of a release added to a collection.
type CollectionInstance struct {
	InstanceID  int    `json:"instance_id"`
	ResourceURL string `json:"resource_url"`

	raw json.RawMessage
}

type collectionInstance CollectionInstance

func (i *CollectionInstance) UnmarshalJSON(b []byte) error {
	return decodeKeep(b, (*collectionInstance)(i), &i.raw)
}
func (i CollectionInstance) MarshalJSON() ([]byte, error) {
	return encodeKept(i.raw, collectionInstance(i))
}

// CollectionInstances returns every copy of the release with id in the
// collection of username, each with its instance ID and folder. An empty
// slice means the user has none. A private collection needs a token for its
// owner.
func (c *Client) CollectionInstances(ctx context.Context, username string, id int) ([]CollectionItem, error) {
	if id < 1 {
		return nil, fmt.Errorf("api: release id %d must be 1 or more", id)
	}
	u, err := c.endpoint("users", username, "collection", "releases", strconv.Itoa(id))
	if err != nil {
		return nil, err
	}
	page, err := getPage[CollectionItem](ctx, c, u, "releases")
	if err != nil {
		return nil, err
	}
	items := []CollectionItem{}
	err = page.Each(ctx, func(item CollectionItem) error {
		items = append(items, item)
		return nil
	})
	return items, err
}

// AddToCollection adds a copy of the release with id to folder in the
// collection of username, which must be the token holder, and returns its new
// instance. folder must be 1 or more; 1 is "Uncategorized". Adding a release
// the user already has adds another copy.
func (c *Client) AddToCollection(ctx context.Context, username string, folder, id int) (*CollectionInstance, error) {
	if folder < 1 {
		return nil, fmt.Errorf("api: folder %d must be 1 or more; 0 holds every release and cannot be added to", folder)
	}
	if id < 1 {
		return nil, fmt.Errorf("api: release id %d must be 1 or more", id)
	}
	u, err := c.endpoint("users", username, "collection", "folders", strconv.Itoa(folder), "releases", strconv.Itoa(id))
	if err != nil {
		return nil, err
	}
	var i CollectionInstance
	if err := c.do(ctx, http.MethodPost, u, nil, &i); err != nil {
		return nil, err
	}
	return &i, nil
}

// RemoveFromCollection removes one copy, instance, of the release with id from
// folder in the collection of username, which must be the token holder.
// CollectionInstances gives each copy's instance and folder.
func (c *Client) RemoveFromCollection(ctx context.Context, username string, folder, id, instance int) error {
	if folder < 1 || id < 1 || instance < 1 {
		return fmt.Errorf("api: folder %d, release id %d and instance %d must each be 1 or more", folder, id, instance)
	}
	u, err := c.endpoint("users", username, "collection", "folders", strconv.Itoa(folder), "releases", strconv.Itoa(id), "instances", strconv.Itoa(instance))
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodDelete, u, nil, nil)
}
