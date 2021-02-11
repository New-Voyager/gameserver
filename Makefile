DEFAULT_DOCKER_NET := poker_test

GCR_REGISTRY := gcr.io/voyager-01-285603
DO_REGISTRY := registry.digitalocean.com/voyager
REGISTRY := $(DO_REGISTRY)

API_SERVER_IMAGE := $(REGISTRY)/api-server:0.2.4
GAME_SERVER_IMAGE := $(REGISTRY)/game-server:0.2.3
BOTRUNNER_IMAGE := $(REGISTRY)/botrunner:0.2.3
NATS_SERVER_IMAGE := $(REGISTRY)/nats:2.1.7-alpine3.11
REDIS_IMAGE := $(REGISTRY)/redis:6.0.9
POSTGRES_IMAGE := $(REGISTRY)/postgres:12.5

SERVER_DIR := server
BOTRUNNER_DIR := botrunner
TEST_DIR := test

.PHONE: login
login:
	docker login --username c1a5bd86f63f2882b8a11671bce3bae92e8355abf6e23613d7758a824c8f5082 --password c1a5bd86f63f2882b8a11671bce3bae92e8355abf6e23613d7758a824c8f5082 registry.digitalocean.com

.PHONY: pull
pull: login	
	docker pull $(API_SERVER_IMAGE)
	docker pull $(GAME_SERVER_IMAGE)
	docker pull $(NATS_SERVER_IMAGE)
	docker pull $(REDIS_IMAGE)
	docker pull $(POSTGRES_IMAGE)
	docker pull $(BOTRUNNER_IMAGE)

.PHONY: create-network
create-network:
	@docker network create $(DEFAULT_DOCKER_NET) 2>/dev/null || true

.PHONY: stack-up
stack-up: create-network login
	> .env && \
		echo "API_SERVER_IMAGE=$(API_SERVER_IMAGE)" >> .env && \
		echo "GAME_SERVER_IMAGE=$(GAME_SERVER_IMAGE)" >> .env && \
		echo "NATS_SERVER_IMAGE=$(NATS_SERVER_IMAGE)" >> .env && \
		echo "REDIS_IMAGE=$(REDIS_IMAGE)" >> .env && \
		echo "POSTGRES_IMAGE=$(POSTGRES_IMAGE)" >> .env && \
		echo "BOTRUNNER_IMAGE=$(BOTRUNNER_IMAGE)" >> .env && \
		echo "PROJECT_ROOT=$(PWD)" >> .env && \
		docker-compose up -d

.PHONY: stack-logs
stack-logs:
	docker-compose logs -f

.PHONY: stack-down
stack-down:
	docker-compose down

.PHONY: compile-proto
compile-proto:
	$(MAKE) -C $(SERVER_DIR) compile-proto
	$(MAKE) -C $(BOTRUNNER_DIR) compile-proto

.PHONY: build
build:
	$(MAKE) -C $(SERVER_DIR) build
	$(MAKE) -C $(BOTRUNNER_DIR) build
	$(MAKE) -C $(TEST_DIR) build

.PHONY: system_test
system_test:
	$(TEST_DIR)/test

# Delegate the other targets to the game server Makefile for now.
.PHONY: %
%:
	$(MAKE) -C $(SERVER_DIR) $@
