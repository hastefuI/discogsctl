# syntax=docker/dockerfile:1

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
