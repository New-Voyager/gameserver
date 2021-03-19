BUILD_NO := $(shell cat build_number.txt)
GCP_PROJECT_ID := voyager-01-285603
DO_REGISTRY := registry.digitalocean.com/voyager
GCP_REGISTRY := gcr.io/$(GCP_PROJECT_ID)
PROTO_VER := v3.7.1
PROTOC_ZIP := protoc-3.7.1-linux-x86_64.zip

DEV_API_SERVER_URL ?= http://localhost:9501
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
	rsync -avu --delete $(PROJECT_ROOT)/gamescript/ build/gamescript
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

