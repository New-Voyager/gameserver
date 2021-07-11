module voyager.com/server

go 1.14

require (
	github.com/MichaelTJones/walk v0.0.0-20161122175330-4748e29d5718 // indirect
	github.com/gin-gonic/gin v1.6.3
	github.com/go-playground/validator/v10 v10.4.1 // indirect
	github.com/go-redis/redis/v8 v8.3.3
	github.com/golang/protobuf v1.5.2
	github.com/hashicorp/golang-lru v0.5.4
	github.com/jmoiron/sqlx v1.3.1
	github.com/json-iterator/go v1.1.10
	github.com/lib/pq v1.10.0
	github.com/mgutz/str v1.2.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.1 // indirect
	github.com/nats-io/nats-server/v2 v2.2.1 // indirect
	github.com/nats-io/nats.go v1.10.1-0.20210330225420-a0b1f60162f8
	github.com/pkg/errors v0.9.1
	github.com/rs/zerolog v1.19.0
	github.com/stretchr/testify v1.6.1
	github.com/ugorji/go v1.2.0 // indirect
	golang.org/x/time v0.0.0-20200630173020-3af7569d3a1e
	google.golang.org/protobuf v1.27.1
	gopkg.in/godo.v2 v2.0.9
	gopkg.in/yaml.v2 v2.3.0
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776
	nhooyr.io/websocket v1.8.6
	voyager.com/encryption v0.0.0-00010101000000-000000000000
)

replace voyager.com/server/internal => ./internal

replace voyager.com/server/poker => ./poker

replace voyager.com/server/game => ./game

replace voyager.com/server/test => ./test

replace voyager.com/server/nats => ./nats

replace voyager.com/server/bot => ./bot

replace voyager.com/server/apiserver => ./apiserver

replace voyager.com/server/rest => ./rest

replace voyager.com/server/timer => ./timer

replace voyager.com/encryption => ../encryption
