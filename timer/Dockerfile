# syntax = docker/dockerfile:1.2

FROM golang:1.13.10-alpine3.11 AS builder

RUN mkdir /build
COPY . /build/timer
COPY build/logging /build/logging
WORKDIR /build/timer
RUN --mount=type=cache,target=/go --mount=type=cache,target=/root/.cache/go-build \
    GOOS=linux \
    GOARCH=amd64 \
    go build -o timer

FROM alpine:3.12.1
RUN apk add --no-cache bash tmux curl
COPY --from=builder /build/timer/timer /app/

WORKDIR /app

CMD ["./timer"]
