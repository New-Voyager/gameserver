BUILD_NO := $(shell cat build_number.txt)
GCP_PROJECT_ID := voyager-01-285603
DO_REGISTRY := registry.digitalocean.com/voyager
GCP_REGISTRY := gcr.io/$(GCP_PROJECT_ID)
REGISTRY := $(DO_REGISTRY)
PROTO_VER := v3.7.1
PROTOC_ZIP := protoc-3.7.1-linux-x86_64.zip

DEV_API_SERVER_URL ?= http://localhost:9501
DEV_POSTGRES_HOST ?= localhost
DEV_POSTGRES_PORT ?= 5432
DEV_POSTGRES_USER ?= game
DEV_POSTGRES_PASSWORD ?= game
DEV_POSTGRES_DB ?= game
PRINT_GAME_MSG ?= false
PRINT_HAND_MSG ?= false

# Script used to drive the botrunner & tester.
DEV_BOT_SCRIPT ?= river-action-3-bots.yaml
# DEV_BOT_SCRIPT ?= river-action-2-bots-1-human.yaml

DOCKER_BUILDKIT ?= 1

PROJECT_ROOT := ..
PROTO_DIR := $(PROJECT_ROOT)/proto
PLAYERS_YAML := $(PWD)/botrunner_scripts/common/players.yaml

.PHONY: install-protoc
install-protoc:
	curl -OL https://github.com/protocolbuffers/protobuf/releases/download/$(PROTO_VER)/$(PROTOC_ZIP)
	sudo unzip -o $(PROTOC_ZIP) -d /usr/local bin/protoc
	sudo unzip -o $(PROTOC_ZIP) -d /usr/local 'include/*'
	rm -f $(PROTOC_ZIP)

.PHONY: compile-proto
compile-proto:
	#go get -u github.com/golang/protobuf/protoc-gen-go
	protoc -I=$(PROTO_DIR) --go_out=./internal/ $(PROTO_DIR)/game.proto
	protoc -I=$(PROTO_DIR) --go_out=./internal/ $(PROTO_DIR)/hand.proto
	protoc -I=$(PROTO_DIR) --go_out=./internal/ $(PROTO_DIR)/gamestate.proto
	protoc -I=$(PROTO_DIR) --go_out=./internal/ $(PROTO_DIR)/handstate.proto
	protoc -I=$(PROTO_DIR) --go_out=./internal/ $(PROTO_DIR)/gamemessage.proto
	protoc -I=$(PROTO_DIR) --go_out=./internal/ $(PROTO_DIR)/handmessage.proto

.PHONY: docker-build
docker-build: compile-proto
	mkdir -p build
	if [ "${CIRCLECI}" = "true" ]; then \
		[ -d build/gamescript ] || cp -r $(PROJECT_ROOT)/gamescript build/gamescript; \
	else \
		rsync -avu --delete $(PROJECT_ROOT)/gamescript/ build/gamescript; \
	fi
	DOCKER_BUILDKIT=$(DOCKER_BUILDKIT) docker build . -t botrunner

.PHONY: build
build: compile-proto
	go build -o botrunner ./cmd/botrunner/main.go
	go build -o tester ./cmd/tester/main.go
	go build -o server ./cmd/server/main.go

.PHONY: run
run:
	API_SERVER_URL=$(DEV_API_SERVER_URL) \
	PRINT_GAME_MSG=$(PRINT_GAME_MSG) \
	PRINT_HAND_MSG=$(PRINT_HAND_MSG) \
	POSTGRES_HOST=$(DEV_POSTGRES_HOST) \
	POSTGRES_PORT=$(DEV_POSTGRES_PORT) \
	POSTGRES_USER=$(DEV_POSTGRES_USER) \
	POSTGRES_PASSWORD=$(DEV_POSTGRES_PASSWORD) \
	POSTGRES_DB=$(DEV_POSTGRES_DB) \
	go run ./cmd/botrunner/main.go --script botrunner_scripts/$(DEV_BOT_SCRIPT)

.PHONY: tester
tester: CODE=$(GAME_CODE)
tester:
	API_SERVER_URL=$(DEV_API_SERVER_URL) \
	PRINT_GAME_MSG=$(PRINT_GAME_MSG) \
	PRINT_HAND_MSG=$(PRINT_HAND_MSG) \
	go run ./cmd/tester/main.go --script botrunner_scripts/$(DEV_BOT_SCRIPT) --game-code $(CODE)

.PHONY: join-human-game
join-human-game: CCODE=$(CLUB_CODE)
join-human-game: GCODE=$(GAME_CODE)
join-human-game:
	API_SERVER_URL=$(DEV_API_SERVER_URL) \
	PRINT_GAME_MSG=$(PRINT_GAME_MSG) \
	PRINT_HAND_MSG=$(PRINT_HAND_MSG) \
	go run ./cmd/botrunner/main.go --script botrunner_scripts/human-game.yaml --club-code $(CCODE) --game-code $(GCODE)

.PHONY: botrunner-server
botrunner-server:
	POSTGRES_HOST=$(DEV_POSTGRES_HOST) \
	POSTGRES_PORT=$(DEV_POSTGRES_PORT) \
	POSTGRES_USER=$(DEV_POSTGRES_USER) \
	POSTGRES_PASSWORD=$(DEV_POSTGRES_PASSWORD) \
	POSTGRES_DB=$(DEV_POSTGRES_DB) \
	API_SERVER_URL=$(DEV_API_SERVER_URL) \
	PRINT_GAME_MSG=$(PRINT_GAME_MSG) \
	PRINT_HAND_MSG=$(PRINT_HAND_MSG) \
	go run ./cmd/server/main.go

.PHONY: publish
publish: do-publish

.PHONY: do-publish
do-publish: export REGISTRY=$(DO_REGISTRY)
do-publish: do-login publish-botrunner

.PHONY: gcp-publish
gcp-publish: export REGISTRY=$(GCP_REGISTRY)
gcp-publish: publish-botrunner

.PHONY: publish-botrunner
publish-botrunner:
	docker tag botrunner $(REGISTRY)/botrunner:$(BUILD_NO)
	docker tag botrunner $(REGISTRY)/botrunner:latest
	docker push $(REGISTRY)/botrunner:$(BUILD_NO)
	docker push $(REGISTRY)/botrunner:latest

.PHONY: do-login
do-login:
	docker login --username 69bf6de23225d8abd358d7c5c2dac07d64a7f6c0bd97d5a5a974847269f99455 --password 69bf6de23225d8abd358d7c5c2dac07d64a7f6c0bd97d5a5a974847269f99455 registry.digitalocean.com




#################
## System Test ##
#################

TEST_DOCKER_NET := poker_test

# These images are downloaded.
API_SERVER_IMAGE := $(REGISTRY)/api-server:0.3.6
NATS_SERVER_IMAGE := $(REGISTRY)/nats:2.1.7-alpine3.11
REDIS_IMAGE := $(REGISTRY)/redis:6.0.9
POSTGRES_IMAGE := $(REGISTRY)/postgres:12.5

# These images are built from local code.
GAME_SERVER_IMAGE=game-server:latest
BOTRUNNER_IMAGE=botrunner:latest

COMPOSE_SYSTEM_TEST := docker-compose -p system_test -f docker-compose-systest.yaml
ifndef CIRCLECI
    COMPOSE_SYSTEM_TEST := $(COMPOSE_SYSTEM_TEST) -f docker-compose-systest-local.yaml
endif

.PHONY: system-test
system-test: system-test-down system-test-up
	docker exec -t system_test_botrunner_1 bash -c "./system_test.sh"

.PHONY: system-test-build
system-test-build:
	$(MAKE) -C $(PROJECT_ROOT)/server docker-build
	$(MAKE) -C $(PROJECT_ROOT)/botrunner docker-build

.PHONY: system-test-down
system-test-down: system-test-generate-env
	$(COMPOSE_SYSTEM_TEST) down

.PHONY: system-test-up
system-test-up: system-test-create-network do-login system-test-generate-env
	$(COMPOSE_SYSTEM_TEST) up -d

.PHONY: system-test-logs
system-test-logs:
	$(COMPOSE_SYSTEM_TEST) logs -f

.PHONY: system-test-create-network
system-test-create-network:
	@docker network create $(TEST_DOCKER_NET) 2>/dev/null || true

.PHONY: system-test-generate-env
system-test-generate-env:
	> .env && \
		echo "API_SERVER_IMAGE=$(API_SERVER_IMAGE)" >> .env && \
		echo "GAME_SERVER_IMAGE=$(GAME_SERVER_IMAGE)" >> .env && \
		echo "BOTRUNNER_IMAGE=$(BOTRUNNER_IMAGE)" >> .env && \
		echo "NATS_SERVER_IMAGE=$(NATS_SERVER_IMAGE)" >> .env && \
		echo "REDIS_IMAGE=$(REDIS_IMAGE)" >> .env && \
		echo "POSTGRES_IMAGE=$(POSTGRES_IMAGE)" >> .env && \
		echo "TEST_DOCKER_NET=$(TEST_DOCKER_NET)" >> .env
