PROTOC_ZIP := protoc-3.7.1-linux-x86_64.zip
GCP_PROJECT_ID := voyager-01-285603
BUILD_NO := $(shell cat build_number.txt)
DEFAULT_DOCKER_NET := game
DO_REGISTRY := registry.digitalocean.com/voyager
GCP_REGISTRY := gcr.io/${GCP_PROJECT_ID}

DEV_REDIS_HOST := localhost
DEV_REDIS_PORT := 6379
DEV_REDIS_DB := 0
DEV_API_SERVER_URL := http://localhost:9501

REDIS_VERSION := 6.0.9

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
	curl -OL https://github.com/protocolbuffers/protobuf/releases/download/v3.7.1/${PROTOC_ZIP}
	sudo unzip -o ${PROTOC_ZIP} -d /usr/local bin/protoc
	sudo unzip -o ${PROTOC_ZIP} -d /usr/local 'include/*'
	rm -f ${PROTOC_ZIP}

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
	docker-compose build

.PHONY: create-network
create-network:
	@docker network create $(DEFAULT_DOCKER_NET) 2>/dev/null || true

.PHONY: run-nats
run-nats: create-network
	docker rm -f nats || true
	docker run -d --name nats --network $(DEFAULT_DOCKER_NET) -p 4222:4222 -p 9222:9222 -p 8222:8222 nats-server

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

.PHONY: run-bot
run-bot: export PERSIST_METHOD=redis
run-bot: export REDIS_HOST=$(DEV_REDIS_HOST)
run-bot: export REDIS_PORT=$(DEV_REDIS_PORT)
run-bot: export REDIS_DB=$(DEV_REDIS_DB)
run-bot:
	go run ./main.go --bot

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

.PHONY: up
up:
	docker-compose -f docker-compose.yaml up

.PHONY: fmt
fmt:
	go fmt
	cd game && go fmt
	cd internal && go fmt
	cd poker && go fmt

.PHONY: publish
publish: export REGISTRY=${GCP_REGISTRY}
publish:  publish-common

.PHONY: do-publish
do-publish: export REGISTRY=${DO_REGISTRY}
do-publish: publish-common

.PHONY: publish-common
publish-common:
	# publish redis and curl so that we don't have to pull from the docker hub
	# curl image is used in helm chart
	docker pull redis:${REDIS_VERSION}
	docker tag redis:${REDIS_VERSION} ${REGISTRY}/redis:${REDIS_VERSION}
	docker push ${REGISTRY}/redis:${REDIS_VERSION}
	docker pull curlimages/curl:7.72.0
	docker tag curlimages/curl:7.72.0 ${REGISTRY}/curlimages/curl:7.72.0
	docker push ${REGISTRY}/curlimages/curl:7.72.0

	# publish nats
	docker tag nats-server ${REGISTRY}/nats-server:$(BUILD_NO)
	docker tag nats-server ${REGISTRY}/nats-server:latest
	docker push ${REGISTRY}/nats-server:$(BUILD_NO)
	docker push ${REGISTRY}/nats-server:latest

	# publish gameserver
	docker tag game-server ${REGISTRY}/game-server:$(BUILD_NO)
	docker tag game-server ${REGISTRY}/game-server:latest
	docker push ${REGISTRY}/game-server:$(BUILD_NO)
	docker push ${REGISTRY}/game-server:latest
