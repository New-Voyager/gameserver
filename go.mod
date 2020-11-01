module voyager.com/server

go 1.14

require (
	github.com/cweill/gotests v1.5.3 // indirect
	github.com/go-redis/redis/v8 v8.3.3
	github.com/golang/protobuf v1.4.3
	github.com/google/uuid v1.1.1
	github.com/json-iterator/go v1.1.10
	github.com/mgechev/revive v1.0.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.1 // indirect
	github.com/nats-io/nats.go v1.10.0
	github.com/rs/zerolog v1.19.0
	github.com/stretchr/objx v0.2.0 // indirect
	github.com/stretchr/testify v1.6.1
	golang.org/x/time v0.0.0-20200630173020-3af7569d3a1e
	google.golang.org/protobuf v1.25.0
	gopkg.in/yaml.v2 v2.3.0
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776 // indirect
	nhooyr.io/websocket v1.8.6
)

replace voyager.com/server/internal => ./internal

replace voyager.com/server/poker => ./poker

replace voyager.com/server/game => ./game

replace voyager.com/server/test => ./test

replace voyager.com/server/nats => ./nats

replace voyager.com/server/bot => ./bot
