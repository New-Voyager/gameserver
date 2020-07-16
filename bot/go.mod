module voyager.com/server/nats/bot

go 1.14

require (
	github.com/json-iterator/go v1.1.10
	github.com/nats-io/nats.go v1.10.0
	github.com/rs/zerolog v1.19.0
	gopkg.in/yaml.v2 v2.3.0
	voyager.com/server/game v0.0.0
	voyager.com/server/test v0.0.0-00010101000000-000000000000
)

replace voyager.com/server/game => ../game

replace voyager.com/server/internal => ../internal

replace voyager.com/server/poker => ../poker

replace voyager.com/server/test => ../test
