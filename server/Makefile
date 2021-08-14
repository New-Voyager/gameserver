PROJECT_ROOT := ..
BUILD_HELPER := $(PROJECT_ROOT)/build_helper

PROTOC_ZIP := protoc-3.7.1-linux-x86_64.zip
BUILD_NO := $(shell cat build_number.txt)
DO_REGISTRY := registry.digitalocean.com/voyager
GCP_PROJECT_ID := voyager-01-285603
GCP_REGISTRY := gcr.io/$(GCP_PROJECT_ID)

DEV_REDIS_HOST := localhost
DEV_REDIS_PORT := 6379
DEV_REDIS_DB := 0
DEV_API_SERVER_URL := http://localhost:9501
DEV_POSTGRES_HOST := localhost
DEV_POSTGRES_PORT := 5432
DEV_POSTGRES_DB := game
DEV_POSTGRES_USER := game
DEV_POSTGRES_PASSWORD := game

DEV_DISABLE_DELAYS ?= 1
DEV_ENABLE_ENCRYPTION ?= 1

NATS_VERSION := 2.1.7-alpine3.11
REDIS_VERSION := 6.0.9

DOCKER_BUILDKIT ?= 1
BUILD_MODE ?= prod
LOCAL_IMAGE_NAME := $(shell $(BUILD_HELPER)/get_local_image_name.sh game-server)
GAME_SERVER_IMAGE ?= $(LOCAL_IMAGE_NAME)
PROTO_DIR := $(PROJECT_ROOT)/proto

TEST_DOCKER_NET := $(shell $(BUILD_HELPER)/get_local_docker_net.sh game_server_docker_test)
COMPOSE_PROJECT_NAME := $(shell $(BUILD_HELPER)/get_compose_project_name.sh game_server_docker_test)
COMPOSE_DOCKER_TEST := docker-compose -p $(COMPOSE_PROJECT_NAME) -f docker-compose-test.yaml

# NATS Logging Options:
# -l, --log <file>                 File to redirect log output
# -T, --logtime                    Timestamp log entries (default: true)
# -s, --syslog                     Log to syslog or windows event log
# -r, --remote_syslog <addr>       Syslog server addr (udp://localhost:514)
# -D, --debug                      Enable debugging output
# -V, --trace                      Trace the raw protocol
# -VV                              Verbose trace (traces system account as well)
# -DV                              Debug and trace
# -DVV                             Debug and verbose trace (traces system account as well)
NATS_OPTS ?= -DV

.PHONY: pull
pull:
	docker pull nats:$(NATS_VERSION)
	docker pull redis:$(REDIS_VERSION)

.PHONY: compile-proto
compile-proto: install-protoc
	go get -u github.com/golang/protobuf/protoc-gen-go
	rm -f game/*.pb.go
	protoc -I=$(PROTO_DIR) --go_out=./ $(PROTO_DIR)/enums.proto
	protoc -I=$(PROTO_DIR) --go_out=./ $(PROTO_DIR)/game.proto
	protoc -I=$(PROTO_DIR) --go_out=./ $(PROTO_DIR)/hand.proto
	protoc -I=$(PROTO_DIR) --go_out=./ $(PROTO_DIR)/handstate.proto
	protoc -I=$(PROTO_DIR) --go_out=./ $(PROTO_DIR)/gamemessage.proto
	protoc -I=$(PROTO_DIR) --go_out=./ $(PROTO_DIR)/handmessage.proto
	ls game/*.pb.go | xargs -n1 -IX bash -c 'sed s/,omitempty// X > X.tmp && mv X{.tmp,}'

.PHONY: install-protoc
install-protoc:
	if ! which protoc; then \
		curl -OL https://github.com/protocolbuffers/protobuf/releases/download/v3.7.1/$(PROTOC_ZIP) && \
		sudo unzip -o $(PROTOC_ZIP) -d /usr/local bin/protoc && \
		sudo unzip -o $(PROTOC_ZIP) -d /usr/local 'include/*' && \
		rm -f $(PROTOC_ZIP); \
	fi

.PHONY: build
build: 
	go build


.PHONY: build2
build2: 
	go build

.PHONY: test
test: export PERSIST_METHOD=memory
test: build
	go test voyager.com/server/poker
	go test voyager.com/server/game

.PHONY: test-build
test-build:
	@# This generates a binary 'game-server.test' that runs the tests when executed.
	go test -coverpkg=./... -c -o game-server.test

.PHONY: script-test
script-test: export PERSIST_METHOD=redis
script-test: export REDIS_HOST=$(DEV_REDIS_HOST)
script-test: export REDIS_PORT=$(DEV_REDIS_PORT)
script-test: export REDIS_DB=$(DEV_REDIS_DB)
script-test: export DISABLE_DELAYS=1
script-test: test-build run-redis
	./game-server.test --script-tests -test.coverprofile=script_test_coverage.out -test.run TestRunMain voyager.com/server
ifdef CI
	go tool cover -html=script_test_coverage.out
else
	go tool cover -html=script_test_coverage.out -o script_test_coverage.html
endif

.PHONY: docker-build
docker-build: compile-proto
	mkdir -p build
	if ! command -v rsync; then \
		[ -d build/encryption ] || cp -r $(PROJECT_ROOT)/encryption build/encryption; \
	else \
		rsync -avu --delete $(PROJECT_ROOT)/encryption/ build/encryption; \
	fi
	DOCKER_BUILDKIT=$(DOCKER_BUILDKIT) docker build . \
		-t $(GAME_SERVER_IMAGE) \
		--build-arg BUILD_MODE=$(BUILD_MODE)
	rm -rf build/

.PHONY: docker-build-test
docker-build-test: export BUILD_MODE=test
docker-build-test: docker-build

.PHONY: create-network
create-network:
	@docker network create $(TEST_DOCKER_NET) 2>/dev/null || true

.PHONY: run-nats
run-nats: create-network
	docker rm -f nats || true
	docker run -d --name nats --network $(TEST_DOCKER_NET) -p 4222:4222 -p 9222:9222 -p 8222:8222 nats:$(NATS_VERSION) --config /etc/nats/nats-server.conf $(NATS_OPTS)

.PHONY: run-redis
run-redis: create-network
	docker rm -f redis || true
	docker run -d --name redis --network $(TEST_DOCKER_NET) -p 6379:6379 redis:$(REDIS_VERSION)

.PHONY: run-server
run-server: export PERSIST_METHOD=redis
run-server: export REDIS_HOST=$(DEV_REDIS_HOST)
run-server: export REDIS_PORT=$(DEV_REDIS_PORT)
run-server: export REDIS_DB=$(DEV_REDIS_DB)
run-server: export API_SERVER_URL=$(DEV_API_SERVER_URL)
run-server: export POSTGRES_HOST=$(DEV_POSTGRES_HOST)
run-server: export POSTGRES_PORT=$(DEV_POSTGRES_PORT)
run-server: export POSTGRES_DB=$(DEV_POSTGRES_DB)
run-server: export POSTGRES_USER=$(DEV_POSTGRES_USER)
run-server: export POSTGRES_PASSWORD=$(DEV_POSTGRES_PASSWORD)
run-server: export ENABLE_ENCRYPTION=$(DEV_ENABLE_ENCRYPTION)
run-server: export DISABLE_DELAYS=$(DEV_DISABLE_DELAYS)
run-server:
	go run ./main.go --server

.PHONY: docker-test-up
docker-test-up: create-network docker-test-generate-env
	$(COMPOSE_DOCKER_TEST) up -d

.PHONY: docker-test-down
docker-test-down: create-network docker-test-generate-env
	timeout 15 $(COMPOSE_DOCKER_TEST) down --timeout 10

.PHONY: docker-test-generate-env
docker-test-generate-env:
	> .env && \
		echo "REDIS_IMAGE=redis:$(REDIS_VERSION)" >> .env && \
		echo "TEST_DOCKER_NET=$(TEST_DOCKER_NET)" >> .env

.PHONY: docker-test
docker-test: docker-test-up
	docker run -t --rm \
		--name $(shell $(BUILD_HELPER)/get_local_container_name.sh gameserver) \
		--network $(TEST_DOCKER_NET) \
		-e PERSIST_METHOD=redis \
		-e REDIS_HOST=redis \
		-e REDIS_PORT=6379 \
		-e REDIS_DB=0 \
		-e DISABLE_DELAYS=1 \
		$(GAME_SERVER_IMAGE) sh -c "/app/game-server --script-tests"

.PHONY: stop
stop:
	docker rm -f nats || true
	docker rm -f redis || true
	docker rm -f game-server || true
	docker network rm $(TEST_DOCKER_NET) || true

.PHONY: fmt
fmt:
	go fmt
	cd game && go fmt
	cd internal && go fmt
	cd poker && go fmt

.PHONY: publish
publish: gcp-publish do-publish

.PHONY: clean-ci
clean-ci: docker-test-down
	docker image rm $(GAME_SERVER_IMAGE) || true
	if docker network ls | grep $(TEST_DOCKER_NET); then docker network rm $(TEST_DOCKER_NET); fi

.PHONY: do-login
do-login:
	@docker login -u 69bf6de23225d8abd358d7c5c2dac07d64a7f6c0bd97d5a5a974847269f99455 -p 69bf6de23225d8abd358d7c5c2dac07d64a7f6c0bd97d5a5a974847269f99455 registry.digitalocean.com

.PHONY: gcp-login
gcp-login:
	@cat $(PROJECT_ROOT)/gcp_dev_image_push.json | docker login -u _json_key --password-stdin https://gcr.io

.PHONY: do-publish
do-publish: export REGISTRY=$(DO_REGISTRY)
do-publish: do-login publish-gameserver

.PHONY: do-publish-all
do-publish-all: export REGISTRY=$(DO_REGISTRY)
do-publish-all: do-login publish-all

.PHONY: gcp-publish
gcp-publish: export REGISTRY=$(GCP_REGISTRY)
gcp-publish: gcp-login publish-gameserver

.PHONY: gcp-publish-all
gcp-publish-all: export REGISTRY=$(GCP_REGISTRY)
gcp-publish-all: gcp-login publish-all

.PHONY: publish-all
publish-all: publish-gameserver publish-3rdparty

.PHONY: publish-gameserver
publish-gameserver:
	docker tag $(GAME_SERVER_IMAGE) $(REGISTRY)/game-server:$(BUILD_NO)
	docker tag $(GAME_SERVER_IMAGE) $(REGISTRY)/game-server:latest
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
