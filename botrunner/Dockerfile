# syntax = docker/dockerfile:1.2

FROM golang:1.13.10-alpine3.11 AS builder

RUN mkdir /build
ADD . /build/
WORKDIR /build
RUN --mount=type=cache,target=/go --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o botrunner ./cmd/botrunner
RUN --mount=type=cache,target=/go --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o tester ./cmd/tester
RUN --mount=type=cache,target=/go --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o server ./cmd/server

FROM alpine:3.12.1
RUN apk add --no-cache bash tmux curl
COPY --from=builder /build/botrunner /app/
COPY --from=builder /build/tester /app/
COPY --from=builder /build/server /app/
COPY --from=builder /build/botrunner_scripts /app/botrunner_scripts
COPY run_many_games.sh /app/

WORKDIR /app

CMD ["./server"]
