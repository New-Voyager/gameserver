# syntax = docker/dockerfile:1.2

FROM golang:1.13.10-alpine3.11 AS builder

RUN mkdir /build
COPY . /build/botrunner
COPY build/gamescript /build/gamescript
COPY build/encryption /build/encryption
COPY build/caching /build/caching
COPY build/logging /build/logging
WORKDIR /build/botrunner
RUN --mount=type=cache,target=/go --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o botrunner ./cmd/botrunner
RUN --mount=type=cache,target=/go --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o tester ./cmd/tester
RUN --mount=type=cache,target=/go --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o server ./cmd/server

FROM alpine:3.12.1
RUN apk add --no-cache bash tmux curl
COPY --from=builder /build/botrunner/botrunner /app/
COPY --from=builder /build/botrunner/tester /app/
COPY --from=builder /build/botrunner/server /app/
COPY --from=builder /build/botrunner/botrunner_scripts /app/botrunner_scripts
COPY run_many_games.sh system_test.sh /app/

WORKDIR /app

CMD ["./server"]
