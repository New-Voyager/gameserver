PROTOC_ZIP := protoc-3.7.1-linux-x86_64.zip
GCP_PROJECT_ID := voyager-01-285603
BUILD_NO := $(shell cat build_number.txt)
DEFAULT_DOCKER_NET := game

DEV_NATS_HOST := localhost
DEV_NATS_CLIENT_PORT := 4222
DEV_REDIS_HOST := localhost
DEV_REDIS_PORT := 6379
DEV_REDIS_DB := 0

.PHONY: compile-proto
compile-proto: install-protoc
	go get -u github.com/golang/protobuf/protoc-gen-go
	protoc -I=./proto --go_out=./game ./proto/gamestate.proto
	protoc -I=./proto --go_out=./game ./proto/handstate.proto
	protoc -I=./proto --go_out=./game ./proto/gamemessage.proto
	protoc -I=./proto --go_out=./game ./proto/handmessage.proto

.PHONY: install-protoc
install-protoc:
	curl -OL https://github.com/protocolbuffers/protobuf/releases/download/v3.7.1/${PROTOC_ZIP}
	sudo unzip -o ${PROTOC_ZIP} -d /usr/local bin/protoc
	sudo unzip -o ${PROTOC_ZIP} -d /usr/local 'include/*'
	rm -f ${PROTOC_ZIP}

.PHONY: build
build: compile-proto
	go build

.PHONY: test
test: export NATS_HOST=$(DEV_NATS_HOST)
test: export NATS_CLIENT_PORT=$(DEV_NATS_CLIENT_PORT)
test: export PERSIST_METHOD=memory
test: build
	go test voyager.com/server/poker
	go test voyager.com/server/game

.PHONY: script-test
script-test: export NATS_HOST=$(DEV_NATS_HOST)
script-test: export NATS_CLIENT_PORT=$(DEV_NATS_CLIENT_PORT)
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
	docker run -d --name nats --network $(DEFAULT_DOCKER_NET) -p 4222:4222 -p 9222:9222 nats-server

.PHONY: run-redis
run-redis: create-network
	docker rm -f redis || true
	docker run -d --name redis --network $(DEFAULT_DOCKER_NET) -p 6379:6379 redis

.PHONY: run-server
run-server: export NATS_HOST=$(DEV_NATS_HOST)
run-server: export NATS_CLIENT_PORT=$(DEV_NATS_CLIENT_PORT)
run-server: export PERSIST_METHOD=redis
run-server: export REDIS_HOST=$(DEV_REDIS_HOST)
run-server: export REDIS_PORT=$(DEV_REDIS_PORT)
run-server: export REDIS_DB=$(DEV_REDIS_DB)
run-server:
	go run ./main.go --server

.PHONY: run-bot
run-bot: export NATS_HOST=$(DEV_NATS_HOST)
run-bot: export NATS_CLIENT_PORT=$(DEV_NATS_CLIENT_PORT)
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
		-e NATS_HOST=nats \
		-e NATS_CLIENT_PORT=4222 \
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
publish:
	# publish nats
	docker tag nats-server gcr.io/${GCP_PROJECT_ID}/nats-server:$(BUILD_NO)
	docker tag nats-server gcr.io/${GCP_PROJECT_ID}/nats-server:latest
	docker push gcr.io/${GCP_PROJECT_ID}/nats-server:$(BUILD_NO)
	docker push gcr.io/${GCP_PROJECT_ID}/nats-server:latest
	# publish gameserver
	docker tag game-server gcr.io/${GCP_PROJECT_ID}/game-server:$(BUILD_NO)
	docker tag game-server gcr.io/${GCP_PROJECT_ID}/game-server:latest
	docker push gcr.io/${GCP_PROJECT_ID}/game-server:$(BUILD_NO)
	docker push gcr.io/${GCP_PROJECT_ID}/game-server:latest
