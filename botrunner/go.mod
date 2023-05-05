module voyager.com/botrunner

go 1.14

require (
	github.com/MichaelTJones/walk v0.0.0-20161122175330-4748e29d5718 // indirect
	github.com/gin-gonic/gin v1.9.0
	github.com/google/go-cmp v0.5.5
	github.com/google/uuid v1.2.0
	github.com/jmoiron/sqlx v1.3.3
	github.com/json-iterator/go v1.1.12
	github.com/lib/pq v1.2.0
	github.com/looplab/fsm v0.2.0
	github.com/machinebox/graphql v0.2.2
	github.com/matryer/is v1.4.0 // indirect
	github.com/mgutz/str v1.2.0 // indirect
	github.com/nats-io/nats-server/v2 v2.1.9 // indirect
	github.com/nats-io/nats.go v1.10.0
	github.com/pkg/errors v0.9.1
	github.com/rs/zerolog v1.25.0
	google.golang.org/protobuf v1.28.1
	gopkg.in/godo.v2 v2.0.9
	voyager.com/caching v0.0.0-00010101000000-000000000000
	voyager.com/encryption v0.0.0-00010101000000-000000000000
	voyager.com/gamescript v0.0.0-00010101000000-000000000000
	voyager.com/logging v0.0.0-00010101000000-000000000000
)

replace voyager.com/gamescript => ../gamescript

replace voyager.com/encryption => ../encryption

replace voyager.com/caching => ../caching

replace voyager.com/logging => ../logging
