module voyager.com/server

go 1.14

require (
	github.com/MichaelTJones/walk v0.0.0-20161122175330-4748e29d5718 // indirect
	github.com/db47h/rand64/v3 v3.1.0
	github.com/gin-gonic/gin v1.6.3
	github.com/go-playground/validator/v10 v10.4.1 // indirect
	github.com/go-redis/redis/v8 v8.3.3
	github.com/golang/protobuf v1.5.2
	github.com/google/go-cmp v0.5.6
	github.com/hashicorp/golang-lru v0.5.4
	github.com/jmoiron/sqlx v1.3.1
	github.com/json-iterator/go v1.1.11
	github.com/lib/pq v1.10.0
	github.com/mgutz/str v1.2.0 // indirect
	github.com/nats-io/nats-server/v2 v2.2.1 // indirect
	github.com/nats-io/nats.go v1.10.1-0.20210330225420-a0b1f60162f8
	github.com/orcaman/concurrent-map v0.0.0-20210501183033-44dafcb38ecc
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.11.0
	github.com/rs/zerolog v1.25.0
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.7.0
	github.com/ugorji/go v1.2.0 // indirect
	golang.org/x/time v0.0.0-20200630173020-3af7569d3a1e // indirect
	google.golang.org/grpc v1.46.2
	google.golang.org/protobuf v1.28.0
	gopkg.in/godo.v2 v2.0.9
	gopkg.in/yaml.v2 v2.3.0
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776
	voyager.com/caching v0.0.0-00010101000000-000000000000
	voyager.com/encryption v0.0.0-00010101000000-000000000000
	voyager.com/logging v0.0.0-00010101000000-000000000000
)

replace voyager.com/server/internal => ./internal

replace voyager.com/server/util => ./util

replace voyager.com/server/poker => ./poker

replace voyager.com/server/game => ./game

replace voyager.com/server/test => ./test

replace voyager.com/server/nats => ./nats

replace voyager.com/server/bot => ./bot

replace voyager.com/server/apiserver => ./apiserver

replace voyager.com/server/rest => ./rest

replace voyager.com/server/timer => ./timer

replace voyager.com/server/rpc => ./rpc

replace voyager.com/server/grpc => ./grpc

replace voyager.com/encryption => ../encryption

replace voyager.com/caching => ../caching

replace voyager.com/logging => ../logging
