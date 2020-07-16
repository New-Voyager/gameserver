PROTOC_ZIP = protoc-3.7.1-linux-x86_64.zip

.PHONY: compile-proto
compile-proto: install-protoc
	go get -u github.com/golang/protobuf/protoc-gen-go
	protoc -I=./proto --go_out=./game ./proto/gamestate.proto
	protoc -I=./proto --go_out=./game ./proto/handstate.proto
	protoc -I=./proto --go_out=./game ./proto/gamemessage.proto
	protoc -I=./proto --go_out=./game ./proto/handmessage.proto

.PHONY: build
build: compile-proto
	go build

.PHONY: fmt
fmt:
	go fmt
	cd game && go fmt
	cd internal && go fmt
	cd poker && go fmt

.PHONY: test
test: build
	go test voyager.com/server/poker
	go test voyager.com/server/game

.PHONY: script-test
script-test:
	go run main.go --script-tests

.PHONY: install-protoc
install-protoc:
	curl -OL https://github.com/protocolbuffers/protobuf/releases/download/v3.7.1/${PROTOC_ZIP}
	sudo unzip -o ${PROTOC_ZIP} -d /usr/local bin/protoc
	sudo unzip -o ${PROTOC_ZIP} -d /usr/local 'include/*'
	rm -f ${PROTOC_ZIP}


.PHONY: run-nats
run-nats:
	cd docker/nats && make build
	cd docker/nats && make run

.PHONY: run-nats
run-nats-no-build:
	cd docker/nats && make run
	