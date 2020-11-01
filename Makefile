PROTOC_ZIP := protoc-3.7.1-linux-x86_64.zip
GCP_PROJECT_ID := voyager-01-285603
BUILD_NO := $(shell cat build_number.txt)

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
test: export NATS_HOST=$(DEV_NATS_HOST)
test: export NATS_CLIENT_PORT=$(DEV_NATS_CLIENT_PORT)
test: export REDIS_HOST=$(DEV_REDIS_HOST)
test: export REDIS_PORT=$(DEV_REDIS_PORT)
test: export REDIS_DB=$(DEV_REDIS_DB)
test: build
	go test voyager.com/server/poker
	go test voyager.com/server/game

.PHONY: script-test
script-test: export NATS_HOST=$(DEV_NATS_HOST)
script-test: export NATS_CLIENT_PORT=$(DEV_NATS_CLIENT_PORT)
script-test: export REDIS_HOST=$(DEV_REDIS_HOST)
script-test: export REDIS_PORT=$(DEV_REDIS_PORT)
script-test: export REDIS_DB=$(DEV_REDIS_DB)
script-test: run-redis
	go run main.go --script-tests

.PHONY: install-protoc
install-protoc:
	curl -OL https://github.com/protocolbuffers/protobuf/releases/download/v3.7.1/${PROTOC_ZIP}
	sudo unzip -o ${PROTOC_ZIP} -d /usr/local bin/protoc
	sudo unzip -o ${PROTOC_ZIP} -d /usr/local 'include/*'
	rm -f ${PROTOC_ZIP}


.PHONY: build-nats
build-nats:
	make -C docker/nats build

.PHONY: run-nats
run-nats: build-nats
	make -C docker/nats run

.PHONY: run-redis
run-redis:
	docker rm -f redis || true
	docker run -d --name redis -p 6379:6379 redis

.PHONY: docker-build
docker-build:
	docker build -f docker/Dockerfile.gameserver . -t game-server

.PHONY: docker-test
docker-test:
	docker run  --name game-server-it game-server /app/game-server --script-tests --game-script /app/game-scripts/

.PHONY: run-nats-server
run-nats-server:
	docker rm -f nats || true
	docker run --network game --name nats -it -p 4222:4222 -p 9222:9222 -d nats:latest

.PHONY: run
run: create-network run-nats-server
	sleep 1
	docker run --network game -it game-server /app/game-server --server --nats-server nats

.PHONY: go-run-server
go-run-server: export NATS_HOST=$(DEV_NATS_HOST)
go-run-server: export NATS_CLIENT_PORT=$(DEV_NATS_CLIENT_PORT)
go-run-server: export REDIS_HOST=$(DEV_REDIS_HOST)
go-run-server: export REDIS_PORT=$(DEV_REDIS_PORT)
go-run-server: export REDIS_DB=$(DEV_REDIS_DB)
go-run-server:
	go run ./main.go --server

.PHONY: go-run-bot
go-run-bot: export NATS_HOST=$(DEV_NATS_HOST)
go-run-bot: export NATS_CLIENT_PORT=$(DEV_NATS_CLIENT_PORT)
go-run-bot: export REDIS_HOST=$(DEV_REDIS_HOST)
go-run-bot: export REDIS_PORT=$(DEV_REDIS_PORT)
go-run-bot: export REDIS_DB=$(DEV_REDIS_DB)
go-run-bot:
	go run ./main.go --bot

.PHONY: run-all
create-network:
	docker rm -f nats || true
	docker rm -f game-server || true
	docker network rm game || true
	docker network create game

stop:
	docker rm -f nats || true
	docker rm -f game-server || true
	docker network rm game || true


.PHONY: up
up:
	docker-compose -f docker-compose.yaml up

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
