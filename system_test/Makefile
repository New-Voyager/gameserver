PROJECT_ROOT := ..

TEST_DOCKER_NET := poker_test
TEST_DOCKER_IMAGE := systest

GCR_REGISTRY := gcr.io/voyager-01-285603
DO_REGISTRY := registry.digitalocean.com/voyager
REGISTRY := $(DO_REGISTRY)

API_SERVER_IMAGE := $(REGISTRY)/api-server:0.2.7
NATS_SERVER_IMAGE := $(REGISTRY)/nats:2.1.7-alpine3.11
REDIS_IMAGE := $(REGISTRY)/redis:6.0.9
POSTGRES_IMAGE := $(REGISTRY)/postgres:12.5

DOCKER_BUILDKIT ?= 1

DEV_REDIS_HOST := localhost
DEV_REDIS_PORT := 6379
DEV_REDIS_DB := 0
DEV_API_SERVER_URL := http://localhost:9501

.PHONY: build
build:
	go build -o test

# Build the docker image used for system test.
.PHONY: docker-build
docker-build:
	# Compile protobuf files for game server and botrunner.
	$(MAKE) -C $(PROJECT_ROOT) compile-proto
	# Build context is the root directory, so that the game server and the botrunner are included.
	DOCKER_BUILDKIT=$(DOCKER_BUILDKIT) docker build $(PROJECT_ROOT) -f ./Dockerfile -t $(TEST_DOCKER_IMAGE)

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
		echo "NATS_SERVER_IMAGE=$(NATS_SERVER_IMAGE)" >> .env && \
		echo "REDIS_IMAGE=$(REDIS_IMAGE)" >> .env && \
		echo "POSTGRES_IMAGE=$(POSTGRES_IMAGE)" >> .env && \
		echo "TEST_DOCKER_NET=$(TEST_DOCKER_NET)" >> .env

.PHONY: stack-up
stack-up: create-network login env
	docker-compose up -d

.PHONY: stack-logs
stack-logs:
	docker-compose logs -f

.PHONY: stack-down
stack-down: env
	docker-compose down

.PHONY: docker-test
docker-test: docker-build stack-down stack-up
	docker run -it --rm \
		--name=systest \
		--network=$(TEST_DOCKER_NET) \
		-e PERSIST_METHOD=redis \
		-e REDIS_HOST=redis \
		-e REDIS_PORT=6379 \
		-e REDIS_DB=0 \
		-e API_SERVER_URL=http://api-server:9501 \
		$(TEST_DOCKER_IMAGE) \
		"./docker-test.sh"

# Run the system test locally.
# The dependent components (api server, nats, redis, postgres) must be launched manually.
.PHONY: test
test: export PERSIST_METHOD=redis
test: export REDIS_HOST=localhost
test: export REDIS_PORT=6379
test: export REDIS_DB=0
test: export API_SERVER_URL=http://localhost:9501
test:
	./test --config config.yaml --game-server-dir=$(PROJECT_ROOT)/server --botrunner-dir=$(PROJECT_ROOT)/botrunner
