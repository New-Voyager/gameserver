PROJECT_ROOT := ..
BUILD_HELPER := $(PROJECT_ROOT)/build_helper

BUILD_NO := $(shell cat build_number.txt)
DO_REGISTRY := registry.digitalocean.com/voyager
GCP_PROJECT_ID := voyager-01-285603
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
ENABLE_ENCRYPTION ?= 1

# Script used to join the human-created game.
HUMAN_GAME_SCRIPT := botrunner_scripts/human_game/human-game.yaml

# Script used to drive the botrunner & tester.
BOTRUNNER_SCRIPT ?= botrunner_scripts/river-action-3-bots.yaml
# BOTRUNNER_SCRIPT ?= botrunner_scripts/human_game/river-action-2-bots-1-human.yaml

DOCKER_BUILDKIT ?= 1
LOCAL_IMAGE_NAME := $(shell $(BUILD_HELPER)/get_local_image_name.sh botrunner)

PROTO_DIR := $(PROJECT_ROOT)/proto
PLAYERS_YAML := $(PWD)/botrunner_scripts/players/default.yaml

.PHONY: install-protoc
install-protoc:
	curl -OL https://github.com/protocolbuffers/protobuf/releases/download/$(PROTO_VER)/$(PROTOC_ZIP)
	sudo unzip -o $(PROTOC_ZIP) -d /usr/local bin/protoc
	sudo unzip -o $(PROTOC_ZIP) -d /usr/local 'include/*'
	rm -f $(PROTOC_ZIP)

.PHONY: compile-proto
compile-proto:
	#go get -u github.com/golang/protobuf/protoc-gen-go
	rm -f internal/game/*.pb.go
	protoc -I=$(PROTO_DIR) --go_out=./internal/ $(PROTO_DIR)/enums.proto
	protoc -I=$(PROTO_DIR) --go_out=./internal/ $(PROTO_DIR)/game.proto
	protoc -I=$(PROTO_DIR) --go_out=./internal/ $(PROTO_DIR)/hand.proto
	protoc -I=$(PROTO_DIR) --go_out=./internal/ $(PROTO_DIR)/handstate.proto
	protoc -I=$(PROTO_DIR) --go_out=./internal/ $(PROTO_DIR)/gamemessage.proto
	protoc -I=$(PROTO_DIR) --go_out=./internal/ $(PROTO_DIR)/handmessage.proto

.PHONY: docker-build
docker-build: compile-proto
	mkdir -p build
	if ! command -v rsync; then \
		[ -d build/gamescript ] || cp -r $(PROJECT_ROOT)/gamescript build/gamescript; \
		[ -d build/encryption ] || cp -r $(PROJECT_ROOT)/encryption build/encryption; \
	else \
		rsync -avu --delete $(PROJECT_ROOT)/gamescript/ build/gamescript; \
		rsync -avu --delete $(PROJECT_ROOT)/encryption/ build/encryption; \
	fi
	DOCKER_BUILDKIT=$(DOCKER_BUILDKIT) docker build . -t $(LOCAL_IMAGE_NAME)
	rm -rf build/

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
	ENABLE_ENCRYPTION=$(ENABLE_ENCRYPTION) \
	go run ./cmd/botrunner/main.go --script $(BOTRUNNER_SCRIPT)

.PHONY: tester
tester: CODE=$(GAME_CODE)
tester:
	API_SERVER_URL=$(DEV_API_SERVER_URL) \
	PRINT_GAME_MSG=$(PRINT_GAME_MSG) \
	PRINT_HAND_MSG=$(PRINT_HAND_MSG) \
	ENABLE_ENCRYPTION=$(ENABLE_ENCRYPTION) \
	go run ./cmd/tester/main.go --script $(BOTRUNNER_SCRIPT) --game-code $(CODE)

.PHONY: join-human-game
join-human-game: CCODE=$(CLUB_CODE)
join-human-game: GCODE=$(GAME_CODE)
join-human-game:
	API_SERVER_URL=$(DEV_API_SERVER_URL) \
	PRINT_GAME_MSG=$(PRINT_GAME_MSG) \
	PRINT_HAND_MSG=$(PRINT_HAND_MSG) \
	ENABLE_ENCRYPTION=$(ENABLE_ENCRYPTION) \
	go run ./cmd/botrunner/main.go --script $(HUMAN_GAME_SCRIPT) --club-code $(CCODE) --game-code $(GCODE)

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
	ENABLE_ENCRYPTION=$(ENABLE_ENCRYPTION) \
	go run ./cmd/server/main.go

.PHONY: publish
publish: gcp-publish do-publish

.PHONY: clean-ci
clean-ci: system-test-down
	docker image rm $(LOCAL_IMAGE_NAME) || true
	if docker network ls | grep $(TEST_DOCKER_NET); then docker network rm $(TEST_DOCKER_NET); fi

.PHONY: do-publish
do-publish: export REGISTRY=$(DO_REGISTRY)
do-publish: do-login publish-botrunner

.PHONY: gcp-publish
gcp-publish: export REGISTRY=$(GCP_REGISTRY)
gcp-publish: gcp-login publish-botrunner

.PHONY: publish-botrunner
publish-botrunner:
	docker tag $(LOCAL_IMAGE_NAME) $(REGISTRY)/botrunner:$(BUILD_NO)
	docker tag $(LOCAL_IMAGE_NAME) $(REGISTRY)/botrunner:latest
	docker push $(REGISTRY)/botrunner:$(BUILD_NO)
	docker push $(REGISTRY)/botrunner:latest

.PHONY: do-login
do-login:
	@docker login -u 69bf6de23225d8abd358d7c5c2dac07d64a7f6c0bd97d5a5a974847269f99455 -p 69bf6de23225d8abd358d7c5c2dac07d64a7f6c0bd97d5a5a974847269f99455 registry.digitalocean.com

.PHONY: gcp-login
gcp-login:
	@cat $(PROJECT_ROOT)/gcp_dev_image_push.json | docker login -u _json_key --password-stdin https://gcr.io




#################
## System Test ##
#################

# These images are downloaded.
API_SERVER_IMAGE := $(REGISTRY)/api-server:0.5.34-bomb-pot
NATS_SERVER_IMAGE := $(REGISTRY)/nats:2.1.7-alpine3.11
REDIS_IMAGE := $(REGISTRY)/redis:6.0.9
POSTGRES_IMAGE := $(REGISTRY)/postgres:12.5

# These images are built from local code.
BOTRUNNER_IMAGE=$(LOCAL_IMAGE_NAME)
GAME_SERVER_IMAGE=$(shell $(BUILD_HELPER)/get_local_image_name.sh game-server)
TIMER_IMAGE=$(shell $(BUILD_HELPER)/get_local_image_name.sh timer)

TEST_DOCKER_NET := $(shell $(BUILD_HELPER)/get_local_docker_net.sh system_test)
COMPOSE_PROJECT_NAME := $(shell $(BUILD_HELPER)/get_compose_project_name.sh system_test)
COMPOSE_SYSTEM_TEST := docker-compose -p $(COMPOSE_PROJECT_NAME) -f docker-compose-systest.yaml
ifndef CI
COMPOSE_SYSTEM_TEST := $(COMPOSE_SYSTEM_TEST) -f docker-compose-systest-local.yaml
endif

.PHONY: system-test
system-test: system-test-down system-test-up
	$(COMPOSE_SYSTEM_TEST) exec -T botrunner bash -c "ABORT_ON_FAIL=$(ABORT_ON_FAIL) ./system_test.sh"

.PHONY: system-test-build
system-test-build:
	$(MAKE) -C $(PROJECT_ROOT)/server docker-build
	$(MAKE) -C $(PROJECT_ROOT)/botrunner docker-build
	$(MAKE) -C $(PROJECT_ROOT)/timer docker-build

.PHONY: system-test-down
system-test-down: system-test-create-network system-test-generate-env
	timeout 15 $(COMPOSE_SYSTEM_TEST) down --timeout 10

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
		echo "TIMER_IMAGE=$(TIMER_IMAGE)" >> .env && \
		echo "NATS_SERVER_IMAGE=$(NATS_SERVER_IMAGE)" >> .env && \
		echo "REDIS_IMAGE=$(REDIS_IMAGE)" >> .env && \
		echo "POSTGRES_IMAGE=$(POSTGRES_IMAGE)" >> .env && \
		echo "TEST_DOCKER_NET=$(TEST_DOCKER_NET)" >> .env
