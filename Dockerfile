# syntax=docker/dockerfile:1

# The release stage is what GoReleaser builds, with --target=release, from a
# context holding the binary it already built at <os>/<arch>/discogsctl. It
# never reaches the Go stage below. GoReleaser reads the base image from the
# last FROM in this file, so both final stages must use the same one.
FROM gcr.io/distroless/static:nonroot AS release

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/discogsctl /usr/local/bin/discogsctl

USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/discogsctl"]

# The stages from here build from source, for a plain `docker build .`, which
# builds the last stage.
FROM golang:1.27.0-alpine3.24 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download && go mod verify

COPY main.go ./
COPY api ./api
COPY cmd ./cmd
COPY dump ./dump
COPY internal ./internal
COPY output ./output

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o /out/discogsctl .

# distroless/static carries the CA certificates the HTTPS calls to Discogs
# need, and no shell, which discogsctl never uses.
FROM gcr.io/distroless/static:nonroot

COPY --from=build /out/discogsctl /usr/local/bin/discogsctl

USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/discogsctl"]
