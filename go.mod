module voyager.com/server

go 1.14

require (
	github.com/golang/protobuf v1.4.2
	google.golang.org/protobuf v1.24.0 // indirect
	voyager.com/server/game v0.0.0
	voyager.com/server/internal v0.0.0
	voyager.com/server/poker v0.0.0
)

replace voyager.com/server/internal => ./internal

replace voyager.com/server/poker => ./poker

replace voyager.com/server/game => ./game
