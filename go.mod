module voyager.com/server

go 1.14

require (
	voyager.com/server/internal v0.0.0
	voyager.com/server/poker v0.0.0
)

replace voyager.com/server/internal => ./internal
replace voyager.com/server/poker => ./poker
