# syntax = docker/dockerfile:1.2

ARG BUILD_MODE

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
WORKDIR /build/server

# This is for the production binary.
FROM builder AS builder-prod
RUN --mount=type=cache,target=/go --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 \
    GOOS=linux \
    go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o game-server

# This is for the test binary that includes test runtime, coverage, etc.
FROM builder AS builder-test
RUN --mount=type=cache,target=/go --mount=type=cache,target=/root/.cache/go-build \
    go test -coverpkg=./... -c -o game-server.test

# Depending on the BUILD_MODE argument, we are choosing either builder-prod
# or builder-test stage to be built. Docker buildkit is smart enough to skip the unused
# stage.
FROM builder-${BUILD_MODE} AS builder-final

FROM alpine:latest
COPY --from=builder-final /usr/local/bin/entr /usr/local/bin/entr
COPY --from=builder-final /build/server/game-server* /app/
COPY --from=builder-final /build/server/test/game-scripts /app/test/game-scripts
COPY ./delays.yaml /app/delays.yaml

WORKDIR /app
CMD ["/app/game-server", "--server"]
