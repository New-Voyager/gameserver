PROJECT_ROOT := ..
BUILD_HELPER := $(PROJECT_ROOT)/build_helper

BUILD_NO := $(shell cat build_number.txt)
GCP_PROJECT_ID := voyager-01-285603
DO_REGISTRY := registry.digitalocean.com/voyager
GCP_REGISTRY := gcr.io/$(GCP_PROJECT_ID)
REGISTRY := $(GCP_REGISTRY)

DEV_API_SERVER_INTERNAL_URL ?= http://localhost:9502
DEV_LOG_LEVEL ?= debug

DOCKER_BUILDKIT ?= 1
LOCAL_IMAGE_NAME := $(shell $(BUILD_HELPER)/get_local_image_name.sh scheduler)
SCHEDULER_IMAGE ?= $(LOCAL_IMAGE_NAME)

.PHONY: docker-build
docker-build:
	mkdir -p build
	if ! command -v rsync; then \
		[ -d build/logging ] || cp -r $(PROJECT_ROOT)/logging build/logging; \
	else \
		rsync -avu --delete $(PROJECT_ROOT)/logging/ build/logging; \
	fi
	DOCKER_BUILDKIT=$(DOCKER_BUILDKIT) docker build . -t $(SCHEDULER_IMAGE) --network host
	rm -rf build/

.PHONY: build
build:
	go build -o scheduler

.PHONY: run
run: export API_SERVER_INTERNAL_URL=$(DEV_API_SERVER_INTERNAL_URL)
run: export LOG_LEVEL=$(DEV_LOG_LEVEL)
run: export POST_PROCESSING_INTERVAL_SEC=600
run: export POST_PROCESSING_TIMEOUT_SEC=120
run: export EXPIRE_GAMES_INTERVAL_SEC=600
run: export EXPIRE_GAMES_TIMEOUT_SEC=120
run: export DATA_RETENTION_INTERVAL_MIN=60
run: export DATA_RETENTION_TIMEOUT_MIN=10
run:
	go run ./main.go

.PHONY: publish
publish: gcp-publish

.PHONY: clean-ci
clean-ci:
	docker image rm $(SCHEDULER_IMAGE) || true

.PHONY: do-publish
do-publish: export REGISTRY=$(DO_REGISTRY)
do-publish: do-login publish-scheduler

.PHONY: gcp-publish
gcp-publish: export REGISTRY=$(GCP_REGISTRY)
gcp-publish: gcp-login publish-scheduler

.PHONY: publish-scheduler
publish-scheduler:
	docker tag $(SCHEDULER_IMAGE) $(REGISTRY)/scheduler:$(BUILD_NO)
	docker tag $(SCHEDULER_IMAGE) $(REGISTRY)/scheduler:latest
	docker push $(REGISTRY)/scheduler:$(BUILD_NO)
	docker push $(REGISTRY)/scheduler:latest

.PHONY: do-login
do-login:
	@docker login -u 69bf6de23225d8abd358d7c5c2dac07d64a7f6c0bd97d5a5a974847269f99455 -p 69bf6de23225d8abd358d7c5c2dac07d64a7f6c0bd97d5a5a974847269f99455 registry.digitalocean.com

.PHONY: gcp-login
gcp-login:
	@cat $(PROJECT_ROOT)/gcp_dev_image_push.json | docker login -u _json_key --password-stdin https://gcr.io

.PHONE: debug-local
debug-local: run
