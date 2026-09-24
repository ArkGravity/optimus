# syntax=docker/dockerfile:1

# Builds the single optimus binary with the web UI embedded. Compose runs its
# migrate, seed and server subcommands as separate services.
# Build context MUST be the repository root.

FROM --platform=$BUILDPLATFORM oven/bun:1.3 AS web
WORKDIR /src/web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ ./
RUN bun run build

FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . ./
COPY --from=web /src/web/dist ./web/dist

ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.Version=${VERSION}" \
      -o /out/optimus ./cmd/optimus

FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates tzdata wget
COPY --from=build /out/optimus /usr/local/bin/optimus
COPY configs/config.yaml /etc/optimus/config.yaml
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/optimus"]
CMD ["server", "-config", "/etc/optimus/config.yaml"]
