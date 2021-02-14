# syntax = docker/dockerfile:1.2

FROM golang:1.15.8-alpine3.13

RUN apk add --no-cache bash curl busybox-extras
COPY . /app
WORKDIR /app/server
RUN --mount=type=cache,target=/go --mount=type=cache,target=/root/.cache/go-build go build -o server .
WORKDIR /app/botrunner
RUN --mount=type=cache,target=/go --mount=type=cache,target=/root/.cache/go-build go build -o botrunner ./cmd/botrunner
WORKDIR /app/system_test
RUN --mount=type=cache,target=/go --mount=type=cache,target=/root/.cache/go-build go build -o test .

WORKDIR /app/system_test
