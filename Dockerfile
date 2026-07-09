FROM golang:1.26-alpine3.23 AS builder

ENV GOCACHE=/root/.cache/go-build

RUN --mount=type=cache,target=/var/cache/apk,sharing=locked \
    --mount=type=cache,target=/var/lib/apk,sharing=locked \
    apk add --no-cache \
        "build-base" \
        "libheif-dev=1.23.0-r0"

WORKDIR /app

RUN --mount=type=cache,target=/go/pkg/mod \
    go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0

COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

RUN sqlc generate

RUN --mount=type=cache,target="/root/.cache/go-build" \
    CGO_ENABLED=1 go build \
        -ldflags="-s -w" \
        -o govd ./cmd/main.go

FROM alpine:3.23 AS runtime

WORKDIR /app

RUN --mount=type=cache,target=/var/cache/apk,sharing=locked \
    --mount=type=cache,target=/var/lib/apk,sharing=locked \
    apk add --no-cache \
        "ffmpeg" \
        "libheif=1.23.0-r0"

COPY --from=builder /app/govd ./govd

ENTRYPOINT ["./govd"]
