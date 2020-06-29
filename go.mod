module voyager.com/server

go 1.14

require (
	github.com/golang/protobuf v1.4.2 // indirect
	github.com/rs/zerolog v1.19.0
	github.com/stretchr/objx v0.2.0 // indirect
	github.com/stretchr/testify v1.6.1 // indirect
	google.golang.org/protobuf v1.25.0 // indirect
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776 // indirect
	voyager.com/server/game v0.0.0
	voyager.com/server/internal v0.0.0
	voyager.com/server/poker v0.0.0
)

replace voyager.com/server/internal => ./internal

replace voyager.com/server/poker => ./poker

replace voyager.com/server/game => ./game
