PROTOC_ZIP := protoc-3.7.1-linux-x86_64.zip
GCP_PROJECT_ID := voyager-01-285603
BUILD_NO := $(shell cat build_number.txt)
DEFAULT_DOCKER_NET := game
DO_REGISTRY := registry.digitalocean.com/voyager
GCP_REGISTRY := gcr.io/$(GCP_PROJECT_ID)

DEV_REDIS_HOST := localhost
DEV_REDIS_PORT := 6379
DEV_REDIS_DB := 0
DEV_API_SERVER_URL := http://localhost:9501

NATS_VERSION := 2.1.7-alpine3.11
REDIS_VERSION := 6.0.9

DOCKER_BUILDKIT ?= 1

.PHONY: pull
pull:
	docker pull nats:$(NATS_VERSION)
	docker pull redis:$(REDIS_VERSION)

.PHONY: compile-proto
compile-proto: install-protoc
	go get -u github.com/golang/protobuf/protoc-gen-go
	protoc -I=./proto --go_out=./game ./proto/gamestate.proto
	protoc -I=./proto --go_out=./game ./proto/handstate.proto
	protoc -I=./proto --go_out=./game ./proto/gamemessage.proto
	protoc -I=./proto --go_out=./game ./proto/handmessage.proto

.PHONY: compile-proto2
compile-proto2:
	go get -u github.com/golang/protobuf/protoc-gen-go
	protoc -I=./proto --go_out=./game ./proto/gamestate.proto
	protoc -I=./proto --go_out=./game ./proto/handstate.proto
	protoc -I=./proto --go_out=./game ./proto/gamemessage.proto
	protoc -I=./proto --go_out=./game ./proto/handmessage.proto
	ls game/*.pb.go | xargs -n1 -IX bash -c 'sed s/,omitempty// X > X.tmp && mv X{.tmp,}'

.PHONY: install-protoc
install-protoc:
	curl -OL https://github.com/protocolbuffers/protobuf/releases/download/v3.7.1/$(PROTOC_ZIP)
	sudo unzip -o $(PROTOC_ZIP) -d /usr/local bin/protoc
	sudo unzip -o $(PROTOC_ZIP) -d /usr/local 'include/*'
	rm -f $(PROTOC_ZIP)

.PHONY: build
build: compile-proto
	go build


.PHONY: build2
build2: compile-proto2
	go build

.PHONY: test
test: export PERSIST_METHOD=memory
test: build
	go test voyager.com/server/poker
	go test voyager.com/server/game

.PHONY: script-test
script-test: export PERSIST_METHOD=redis
script-test: export REDIS_HOST=$(DEV_REDIS_HOST)
script-test: export REDIS_PORT=$(DEV_REDIS_PORT)
script-test: export REDIS_DB=$(DEV_REDIS_DB)
script-test: run-redis
	go run main.go --script-tests

.PHONY: docker-build
docker-build: compile-proto
	DOCKER_BUILDKIT=$(DOCKER_BUILDKIT) docker build . -t game-server

.PHONY: create-network
create-network:
	@docker network create $(DEFAULT_DOCKER_NET) 2>/dev/null || true

.PHONY: run-nats
run-nats: create-network
	docker rm -f nats || true
	docker run -d --name nats --network $(DEFAULT_DOCKER_NET) -p 4222:4222 -p 9222:9222 -p 8222:8222 nats:$(NATS_VERSION)

.PHONY: run-redis
run-redis: create-network
	docker rm -f redis || true
	docker run -d --name redis --network $(DEFAULT_DOCKER_NET) -p 6379:6379 redis:$(REDIS_VERSION)

.PHONY: run-server
run-server: export PERSIST_METHOD=redis
run-server: export REDIS_HOST=$(DEV_REDIS_HOST)
run-server: export REDIS_PORT=$(DEV_REDIS_PORT)
run-server: export REDIS_DB=$(DEV_REDIS_DB)
run-server: export API_SERVER_URL=$(DEV_API_SERVER_URL)
run-server:
	go run ./main.go --server

.PHONY: docker-test
docker-test: create-network run-nats run-redis
	docker run -t --rm \
		--name gameserver \
		--network $(DEFAULT_DOCKER_NET) \
		-e REDIS_HOST=redis \
		-e REDIS_PORT=6379 \
		-e REDIS_DB=0 \
		game-server sh -c "PERSIST_METHOD=redis /app/game-server --script-tests && PERSIST_METHOD=memory /app/game-server --script-tests"

.PHONY: stop
stop:
	docker rm -f nats || true
	docker rm -f redis || true
	docker rm -f game-server || true
	docker network rm $(DEFAULT_DOCKER_NET) || true

.PHONY: fmt
fmt:
	go fmt
	cd game && go fmt
	cd internal && go fmt
	cd poker && go fmt

.PHONY: publish
publish: do-publish

.PHONY: do-login
do-login:
	docker login --username 69bf6de23225d8abd358d7c5c2dac07d64a7f6c0bd97d5a5a974847269f99455 --password 69bf6de23225d8abd358d7c5c2dac07d64a7f6c0bd97d5a5a974847269f99455 registry.digitalocean.com

.PHONY: do-publish
do-publish: export REGISTRY=$(DO_REGISTRY)
do-publish: do-login publish-gameserver

.PHONY: do-publish-all
do-publish-all: export REGISTRY=$(DO_REGISTRY)
do-publish-all: do-login publish-all

.PHONY: gcp-publish
gcp-publish: export REGISTRY=$(GCP_REGISTRY)
gcp-publish: publish-gameserver

.PHONY: gcp-publish-all
gcp-publish-all: export REGISTRY=$(GCP_REGISTRY)
gcp-publish-all: publish-all

.PHONY: publish-all
publish-all: publish-gameserver publish-3rdparty

.PHONY: publish-gameserver
publish-gameserver:
	docker tag game-server $(REGISTRY)/game-server:$(BUILD_NO)
	docker tag game-server $(REGISTRY)/game-server:latest
	docker push $(REGISTRY)/game-server:$(BUILD_NO)
	docker push $(REGISTRY)/game-server:latest

.PHONY: publish-3rdparty
publish-3rdparty:
	# publish 3rd-party images so that we don't have to pull from the docker hub
	docker pull redis:$(REDIS_VERSION)
	docker tag redis:$(REDIS_VERSION) $(REGISTRY)/redis:$(REDIS_VERSION)
	docker push $(REGISTRY)/redis:$(REDIS_VERSION)
	docker pull nats:$(NATS_VERSION)
	docker tag nats:$(NATS_VERSION) $(REGISTRY)/nats:$(NATS_VERSION)
	docker push $(REGISTRY)/nats:$(NATS_VERSION)
	docker pull curlimages/curl:7.72.0
	docker tag curlimages/curl:7.72.0 $(REGISTRY)/curlimages/curl:7.72.0
	docker push $(REGISTRY)/curlimages/curl:7.72.0
