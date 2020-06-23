compile-proto:
	protoc -I=./proto --go_out=./game ./proto/gamestate.proto

build: compile-proto
	go build

fmt:
	go fmt
	cd game && go fmt
	cd internal && go fmt
	cd poker && go fmt
