module voyager.com/botrunner

go 1.14

require (
	github.com/MichaelTJones/walk v0.0.0-20161122175330-4748e29d5718 // indirect
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/gin-gonic/gin v1.6.3
	github.com/golang/protobuf v1.4.3 // indirect
	github.com/google/go-cmp v0.5.4
	github.com/jmoiron/sqlx v1.3.3
	github.com/json-iterator/go v1.1.10
	github.com/lib/pq v1.2.0
	github.com/looplab/fsm v0.2.0
	github.com/machinebox/graphql v0.2.2
	github.com/matryer/is v1.4.0 // indirect
	github.com/mgutz/str v1.2.0 // indirect
	github.com/nats-io/nats-server/v2 v2.1.9 // indirect
	github.com/nats-io/nats.go v1.10.0
	github.com/pkg/errors v0.9.1
	github.com/rs/zerolog v1.19.0
	google.golang.org/protobuf v1.25.0
	gopkg.in/godo.v2 v2.0.9
	gopkg.in/yaml.v2 v2.4.0 // indirect
	voyager.com/gamescript v0.0.0-00010101000000-000000000000
)

replace voyager.com/gamescript => ../gamescript
