# syntax = docker/dockerfile:1.2

FROM golang:1.13.10-alpine3.11 AS builder

RUN mkdir /build
COPY . /build
WORKDIR /build
RUN --mount=type=cache,target=/go --mount=type=cache,target=/root/.cache/go-build go build -o scheduler

FROM alpine:3.12.1
RUN apk add --no-cache bash tmux curl
COPY --from=builder /build/scheduler /app/

WORKDIR /app

CMD ["./scheduler"]
