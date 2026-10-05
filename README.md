# discogsctl [![Build](https://github.com/hastefuI/discogsctl/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/hastefuI/discogsctl/actions/workflows/ci.yml) [![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev) [![License](https://img.shields.io/badge/License-MIT-blue.svg)](./LICENSE)

A CLI for [Discogs](https://www.discogs.com) written in Go.

## Overview

[Discogs](https://www.discogs.com) is a music discovery and record collecting platform.

## Features

- **Database**: Get releases, masters, artists and labels, and list their
  versions and releases
- **Search**: Query the database by artist, title, label, year, barcode,
  catalogue number and more
- **Collection**: List folders and the releases in them, and see the estimated
  value of your collection
- **Wantlist**: List the releases in a wantlist
- **Marketplace**: See how many copies of a release are for sale and the
  lowest price, in any supported currency
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
  search [query]          # Search the Discogs database
  release get <id>        # Get a release
  release rating <id>     # Get the community rating of a release
  master get <id>         # Get a master release
  master versions <id>    # List the releases under a master
  artist get <id>         # Get an artist
  artist releases <id>    # List the releases and masters of an artist
  label get <id>          # Get a label
  label releases <id>     # List the releases on a label
  collection folders      # List the folders in a collection
  collection list         # List the releases in a collection folder
  collection value        # Show the estimated value of a collection
  wantlist list           # List the releases in a wantlist
  marketplace stats <id>  # Get the copies for sale and lowest price of a release
  whoami                  # Show the user the token belongs to
  user get <username>     # Get a user's profile
  dump list               # List the data dumps published at data.discogs.com
  dump fetch [id]         # Download a data dump and verify its checksums
  version                 # Print the version and User-Agent

Use "discogsctl [command] --help" for more information about a command.
```

The collection and wantlist commands take `--username`, which defaults to the
token holder. `collection list` also takes `--folder`, which defaults to 0, the
folder that holds every release.

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

# Count the releases in your collection
$ discogsctl collection list --all --output json | jq 'length'

# List your collection as "Artist - Title"
$ discogsctl collection list --all --output json | jq -r '.[].basic_information | "\(.artists[0].name) - \(.title)"'

# Back up your wantlist
$ discogsctl wantlist list --all --output json > wantlist.json

# Get the lowest price of a release in pounds
$ discogsctl marketplace stats 249504 --currency GBP --output json | jq '.lowest_price.value'

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
}
```

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
