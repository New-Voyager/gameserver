module voyager.com/botrunner

go 1.14

require (
	github.com/MichaelTJones/walk v0.0.0-20161122175330-4748e29d5718 // indirect
	github.com/coreos/go-systemd v0.0.0-20190321100706-95778dfbb74e // indirect
	github.com/gin-gonic/gin v1.6.3
	github.com/golang/protobuf v1.4.3 // indirect
	github.com/google/go-cmp v0.5.4
	github.com/google/uuid v1.2.0
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
	github.com/rs/zerolog v1.25.0
	golang.org/x/sys v0.0.0-20210510120138-977fb7262007 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	google.golang.org/protobuf v1.25.0
	gopkg.in/godo.v2 v2.0.9
	gopkg.in/yaml.v2 v2.4.0 // indirect
	voyager.com/caching v0.0.0-00010101000000-000000000000
	voyager.com/encryption v0.0.0-00010101000000-000000000000
	voyager.com/gamescript v0.0.0-00010101000000-000000000000
	voyager.com/logging v0.0.0-00010101000000-000000000000
)

replace voyager.com/gamescript => ../gamescript

replace voyager.com/encryption => ../encryption

replace voyager.com/caching => ../caching

replace voyager.com/logging => ../logging
