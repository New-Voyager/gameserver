PROJECT_ROOT := ..

TEST_DOCKER_NET := poker_test
TEST_DOCKER_IMAGE := systest

GCR_REGISTRY := gcr.io/voyager-01-285603
DO_REGISTRY := registry.digitalocean.com/voyager
REGISTRY := $(DO_REGISTRY)

API_SERVER_IMAGE := $(REGISTRY)/api-server:0.2.29
NATS_SERVER_IMAGE := $(REGISTRY)/nats:2.1.7-alpine3.11
REDIS_IMAGE := $(REGISTRY)/redis:6.0.9
POSTGRES_IMAGE := $(REGISTRY)/postgres:12.5

# These images are built locally.
GAME_SERVER_IMAGE=game-server:latest
BOTRUNNER_IMAGE=botrunner:latest

DOCKER_BUILDKIT ?= 1

DEV_REDIS_HOST := localhost
DEV_REDIS_PORT := 6379
DEV_REDIS_DB := 0
DEV_API_SERVER_URL := http://localhost:9501

ifndef CIRCLECI
    COMPOSE := docker-compose -f docker-compose.yaml -f docker-compose-local.yaml
else
    COMPOSE := docker-compose -f docker-compose.yaml
endif

.PHONY: build
build:
	go build -o test

.PHONY: docker-build
docker-build:
	$(MAKE) -C $(PROJECT_ROOT)/server docker-build
	$(MAKE) -C $(PROJECT_ROOT)/botrunner docker-build

.PHONY: create-network
create-network:
	@docker network create $(TEST_DOCKER_NET) 2>/dev/null || true

.PHONY: login
login:
	docker login --username c1a5bd86f63f2882b8a11671bce3bae92e8355abf6e23613d7758a824c8f5082 --password c1a5bd86f63f2882b8a11671bce3bae92e8355abf6e23613d7758a824c8f5082 registry.digitalocean.com

.PHONY: env
env:
	> .env && \
		echo "API_SERVER_IMAGE=$(API_SERVER_IMAGE)" >> .env && \
		echo "GAME_SERVER_IMAGE=$(GAME_SERVER_IMAGE)" >> .env && \
		echo "BOTRUNNER_IMAGE=$(BOTRUNNER_IMAGE)" >> .env && \
		echo "NATS_SERVER_IMAGE=$(NATS_SERVER_IMAGE)" >> .env && \
		echo "REDIS_IMAGE=$(REDIS_IMAGE)" >> .env && \
		echo "POSTGRES_IMAGE=$(POSTGRES_IMAGE)" >> .env && \
		echo "TEST_DOCKER_NET=$(TEST_DOCKER_NET)" >> .env

.PHONY: stack-up
stack-up: create-network login env
	$(COMPOSE) up -d

.PHONY: stack-logs
stack-logs:
	$(COMPOSE) logs -f

.PHONY: stack-down
stack-down: env
	$(COMPOSE) down

.PHONY: system-test
system-test: docker-build stack-down stack-up
	./docker-test.sh

.PHONY: system-test-logs
system-test-logs:
	$(COMPOSE) logs -f
