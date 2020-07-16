module voyager.com/game

go 1.14

require (
	github.com/golang/protobuf v1.4.1
	github.com/json-iterator/go v1.1.9
	github.com/rs/zerolog v1.19.0
	github.com/stretchr/objx v0.2.0 // indirect
	github.com/stretchr/testify v1.6.1
	google.golang.org/protobuf v1.25.0
	gopkg.in/yaml.v2 v2.2.8
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776 // indirect
	voyager.com/server/internal v0.0.0
	voyager.com/server/poker v0.0.0
)

replace voyager.com/server/internal => ../internal

replace voyager.com/server/poker => ../poker

replace voyager.com/server/test => ../test
