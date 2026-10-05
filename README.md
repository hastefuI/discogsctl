# discogsctl [![Build](https://github.com/hastefuI/discogsctl/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/hastefuI/discogsctl/actions/workflows/ci.yml) [![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev) [![License](https://img.shields.io/badge/License-MIT-blue.svg)](./LICENSE)

A CLI for [Discogs](https://www.discogs.com) written in Go.

## Overview

[Discogs](https://www.discogs.com) is a music discovery and record collecting platform.

## Features

- **Database**: Get releases, masters, artists and labels, and list their
  versions and releases
- **Search**: Query the database by artist, title, label, year, barcode,
  catalogue number and more
- **Collection**: List folders and the releases in them, see the estimated
  value of your collection, and add and remove releases in your own
- **Wantlist**: List the releases in a wantlist, and add, annotate and remove
  releases in your own
- **Marketplace**: See how many copies of a release are for sale and the
  lowest price in any supported currency, get suggested prices by condition,
  and list a seller's inventory and your orders as a seller
- **Export**: Back up a collection and wantlist as one JSON file
- **Data dumps**: List and download the monthly [data dumps](https://data.discogs.com),
  each file verified against its published SHA-256

## Installation

### Build From Source

`discogsctl` requires Go 1.27 or newer.

Clone this repository, and then:

```bash
# Build
$ go build -o discogsctl .

# Install
$ go install .
```

### Docker

Build:
```bash
$ docker build -t discogsctl .
```

Run:
```bash
$ docker run --rm -e DISCOGSCTL_TOKEN discogsctl whoami
```

Download data dumps into a mounted directory, since the image has none it can
write to:
```bash
$ docker run --rm -v "$PWD/dumps:/data" discogsctl dump fetch --latest --type labels --dir /data
```

### Verify Installation

Verify that the installation for discogsctl was successful:

```bash
$ discogsctl --version
discogsctl dev
```

## Quick Start

### Prerequisites

A Discogs account is required for anything beyond public database reads.

### Authenticating with Discogs

`discogsctl` authenticates with a personal access token following the [Discogs API Authentication Guidelines](https://www.discogs.com/developers#page:authentication,header:authentication-discogs-auth-flow), which can be generated
from the [Discogs Developer Settings](https://www.discogs.com/settings/developers)
after login.

Verify that the token works by asking Discogs who it belongs to:

```bash
$ export DISCOGSCTL_TOKEN=<replace-me>
$ discogsctl whoami
```

Without a token, requests are limited to 25 a minute, image URLs are left out,
and `whoami`, private collections and collection value are unavailable.

### First Request

```bash
$ discogsctl release get 249504
ID:       249504
Title:    Never Gonna Give You Up
Artists:  Rick Astley
Released: 1987-07-00
Country:  UK
Labels:   RCA (PB 41447)
Formats:  Vinyl, 7", 45 RPM, Single, Stereo
Genres:   Electronic, Pop
Styles:   Euro-Disco
Master:   96559
Rating:   3.86 (242 votes)
Have:     4132
Want:     589
For sale: 119, from 0.66
URL:      https://www.discogs.com/release/249504-Rick-Astley-Never-Gonna-Give-You-Up

Tracklist:
  A  Never Gonna Give You Up                 3:32
  B  Never Gonna Give You Up (Instrumental)  3:30
```

## Usage

```bash
discogsctl [command] [flags]

Available Commands:
  search [query]             # Search the Discogs database
  release get <id>           # Get a release
  release rating <id>        # Get the community rating of a release
  release rate <id>          # Rate a release from 1 to 5
  release unrate <id>        # Remove your rating of a release
  master get <id>            # Get a master release
  master versions <id>       # List the releases under a master
  artist get <id>            # Get an artist
  artist releases <id>       # List the releases and masters of an artist
  label get <id>             # Get a label
  label releases <id>        # List the releases on a label
  collection folders         # List the folders in a collection
  collection list            # List the releases in a collection folder
  collection value           # Show the estimated value of a collection
  collection add <id>        # Add a release to your collection
  collection remove <id>     # Remove a release from your collection
  wantlist list              # List the releases in a wantlist
  wantlist add <id>          # Add a release to your wantlist
  wantlist edit <id>         # Change your notes on a release in your wantlist
  wantlist remove <id>       # Remove a release from your wantlist
  marketplace stats <id>     # Get the copies for sale and lowest price of a release
  marketplace price <id>     # Get suggested prices for a release by condition
  marketplace orders         # List your marketplace orders as a seller
  marketplace order <id>     # Get one of your orders with its items and tracking
  marketplace messages <id>  # List the messages and history of one of your orders
  marketplace inventory      # List a seller's listings, your own by default
  export                     # Export a collection and wantlist as JSON
  whoami                     # Show the user the token belongs to
  user get <username>        # Get a user's profile
  dump list                  # List the data dumps published at data.discogs.com
  dump fetch [id]            # Download a data dump and verify its checksums
  version                    # Print the version and User-Agent

Use "discogsctl [command] --help" for more information about a command.
```

## Examples

```bash
# Print the tracklist of a release
$ discogsctl release get 249504 --output json | jq '.tracklist[].title'

# Search for masters by an artist
$ discogsctl search --type master --artist "Bjork" --output json | jq -r '.[].title'

# Search by barcode
$ discogsctl search --barcode "7 2064-24425-2 4" --type release

# List the CD versions of a master with their country and catalogue number
$ discogsctl master versions 1000 --all --output json | jq -r '.[] | select(.major_formats | index("CD")) | "\(.id) \(.country) \(.catno)"'

# Add a release to your collection, then remove it
$ discogsctl collection add 5077187
$ discogsctl collection remove 5077187

# Count the releases in your collection
$ discogsctl collection list --all --output json | jq 'length'

# List your collection as "Artist - Title"
$ discogsctl collection list --all --output json | jq -r '.[].basic_information | "\(.artists[0].name) - \(.title)"'

# Back up your wantlist
$ discogsctl wantlist list --all --output json > wantlist.json

# Back up your collection and wantlist together
$ discogsctl export > discogs-backup.json

# Rate a release, then remove the rating
$ discogsctl release rate 8191071 --rating 5
$ discogsctl release unrate 8191071

# Add a release to your wantlist with a note, then change the note
$ discogsctl wantlist add 8191071 --notes "first press only"
$ discogsctl wantlist edit 8191071 --notes "any pressing"

# Get the lowest price of a release in pounds
$ discogsctl marketplace stats 249504 --currency GBP --output json | jq '.lowest_price.value'

# Get the suggested price of a release in Near Mint condition
$ discogsctl marketplace price 8191071 --output json | jq '."Near Mint (NM or M-)".value'

# Count your orders by status
$ discogsctl marketplace orders --all --output json | jq 'group_by(.status) | map({(.[0].status): length}) | add'

# List the orders paid for but not yet shipped
$ discogsctl marketplace orders --status "Payment Received"

# Read the conversation on an order, without the status and payment entries
$ discogsctl marketplace messages <order_id> --all --output json | jq -r '.[] | select(.type == "message") | "\(.timestamp) \(.from.username): \(.message)"'

# List a seller's most expensive listings
$ discogsctl marketplace inventory <username> --sort price --sort-order desc

# Count your own listings by status
$ discogsctl marketplace inventory --all --output json | jq 'group_by(.status) | map({(.[0].status): length}) | add'

# Show your newest order with its items, shipping and tracking
$ discogsctl marketplace order "$(discogsctl marketplace orders --sort-order desc --per-page 1 --output json | jq -r '.[0].id')"

# Back up your wantlist using Docker
$ docker run --rm -e DISCOGSCTL_TOKEN discogsctl wantlist list --all --output json > wantlist.json

# Read someone else's public collection
$ discogsctl collection list --username <username> --all --output json

# List the latest data dumps
$ discogsctl dump list

# Download the newest releases dump into ./dumps, verified against its checksum
$ discogsctl dump fetch --latest --type releases --dir ./dumps

# Download a dump by the start of its ID; after a failure, run it again to fetch only what is missing
$ discogsctl dump fetch 202609 --type labels,masters

# Print the download URL of the releases file in one dump
$ discogsctl dump list --output json | jq -r '.[] | select(.id == "20261001") | .files[] | select(.type == "releases") | .url'
```

## Library

The Discogs API client is importable without the CLI:

```bash
$ go get go.hasteful.org/discogsctl/api
```

Discogs identifies the application, not the library, so a caller passes its
own User-Agent. `api.New` rejects an empty one, and one that names `curl`, a
browser or another HTTP library.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"go.hasteful.org/discogsctl/api"
)

func main() {
	client, err := api.New(api.Options{
		UserAgent: "myapp/0.1 (+https://example.com)",
		Token:     os.Getenv("DISCOGSCTL_TOKEN"),
	})
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	release, err := client.Release(ctx, 249504)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(release.Title, release.Year)

	// Each follows pagination through the client's rate limiter.
	versions, err := client.MasterVersions(ctx, release.MasterID, api.Page{PerPage: 100})
	if err != nil {
		log.Fatal(err)
	}
	err = versions.Each(ctx, func(v api.MasterVersion) error {
		fmt.Println(v.ID, v.Country, v.Format)
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	// RateLimit is the X-Discogs-Ratelimit headers from the last response.
	rl := client.RateLimit()
	fmt.Println(rl.Remaining, "of", rl.Limit, "requests left this minute")
}
```

The client throttles itself to stay within the Discogs rate limit, so a caller
does not need to. `RateLimit` reports the limit, used and remaining counts
Discogs last sent, for showing the budget or deciding when to start a long walk.

A response with a status of 400 or above comes back as an `*api.Error` with the
status code and the Discogs message. Encoding any value the client returns with
`encoding/json` gives back the body as Discogs sent it, including fields the Go
type does not name.

## Development

### Testing

```bash
$ go test -race ./...
```

## Disclaimer

This is an unofficial CLI not affiliated with or endorsed by Discogs. Use of the
Discogs API is subject to the [API Terms of Use](https://support.discogs.com/hc/articles/360009334593-API-Terms-of-Use).

## License

Licensed under the [MIT License](https://opensource.org/licenses/MIT). See [LICENSE](./LICENSE) for details.

Copyright (c) 2026-present hasteful
