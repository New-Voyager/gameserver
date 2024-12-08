# syntax = docker/dockerfile:1.2

FROM golang:1.13.10-alpine3.11 AS builder

RUN wget https://github.com/eradman/entr/archive/4.6.tar.gz -O entr.tar.gz && \
    tar -xvf entr.tar.gz && \
    cd entr-4.6/ && \
    apk add --no-cache build-base gcc && \
    ./configure && make install && \
    find /usr/local/bin/entr
RUN mkdir /build
COPY . /build/server
COPY build/encryption /build/encryption
COPY build/caching /build/caching
COPY build/logging /build/logging
WORKDIR /build/server

RUN --mount=type=cache,target=/go --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o game-server
RUN --mount=type=cache,target=/go --mount=type=cache,target=/root/.cache/go-build \
    GOOS=linux \
    GOARCH=amd64 \
    go test -coverpkg=./... -c -o game-server.test

FROM alpine:latest
COPY --from=builder /usr/local/bin/entr /usr/local/bin/entr
COPY --from=builder /build/server/game-server* /app/
COPY --from=builder /build/server/test/game-scripts /app/test/game-scripts
COPY ./delays.yaml /app/delays.yaml
COPY ./entrypoint*.sh /app/
RUN mkdir /app/code_coverage

WORKDIR /app
CMD ["/app/entrypoint.sh", "--server"]
