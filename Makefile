compile-proto:
	protoc -I=./proto --go_out=./game ./proto/gamestate.proto

build: compile-proto
	go build
