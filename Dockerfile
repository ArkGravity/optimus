# syntax=docker/dockerfile:1

# One image contains the single optimus binary. Compose runs its migrate,
# seed and server subcommands as separate services.
# Build context MUST be the repository root.

FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . ./

ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    mkdir -p /out && \
    CGO_ENABLED=0 go build -ldflags "-s -w -X main.Version=${VERSION}" \
      -o /out/optimus ./cmd/optimus

FROM alpine:3.20 AS backend
RUN apk add --no-cache ca-certificates tzdata wget
COPY --from=build /out/optimus /usr/local/bin/optimus
COPY configs/config.yaml /etc/optimus/config.yaml
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/optimus"]
CMD ["server", "-config", "/etc/optimus/config.yaml"]
