module voyager.com/scheduler

go 1.14

require (
	github.com/gin-gonic/gin v1.9.0
	github.com/hashicorp/golang-lru v0.5.4
	github.com/pkg/errors v0.9.1
	github.com/rs/zerolog v1.25.0
	voyager.com/logging v0.0.0-00010101000000-000000000000
)

replace voyager.com/logging => ../logging
