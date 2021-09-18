PROJECT_ROOT := ..
BUILD_HELPER := $(PROJECT_ROOT)/build_helper

BUILD_NO := $(shell cat build_number.txt)
GCP_PROJECT_ID := voyager-01-285603
DO_REGISTRY := registry.digitalocean.com/voyager
GCP_REGISTRY := gcr.io/$(GCP_PROJECT_ID)
REGISTRY := $(GCP_REGISTRY)

DEV_API_SERVER_URL ?= http://localhost:9501
DEV_LOG_LEVEL ?= info

DOCKER_BUILDKIT ?= 1
LOCAL_IMAGE_NAME := $(shell $(BUILD_HELPER)/get_local_image_name.sh timer)
TIMER_IMAGE ?= $(LOCAL_IMAGE_NAME)

.PHONY: docker-build
docker-build:
	DOCKER_BUILDKIT=$(DOCKER_BUILDKIT) docker build . -t $(TIMER_IMAGE)

.PHONY: build
build:
	go build -o timer

.PHONY: run
run: export API_SERVER_URL=$(DEV_API_SERVER_URL)
run: export LOG_LEVEL=$(DEV_LOG_LEVEL)
run:
	go run ./main.go

.PHONY: publish
publish: gcp-publish

.PHONY: clean-ci
clean-ci:
	docker image rm $(TIMER_IMAGE) || true

.PHONY: do-publish
do-publish: export REGISTRY=$(DO_REGISTRY)
do-publish: do-login publish-timer

.PHONY: gcp-publish
gcp-publish: export REGISTRY=$(GCP_REGISTRY)
gcp-publish: gcp-login publish-timer

.PHONY: publish-timer
publish-timer:
	docker tag $(TIMER_IMAGE) $(REGISTRY)/timer:$(BUILD_NO)
	docker tag $(TIMER_IMAGE) $(REGISTRY)/timer:latest
	docker push $(REGISTRY)/timer:$(BUILD_NO)
	docker push $(REGISTRY)/timer:latest

.PHONY: do-login
do-login:
	@docker login -u 69bf6de23225d8abd358d7c5c2dac07d64a7f6c0bd97d5a5a974847269f99455 -p 69bf6de23225d8abd358d7c5c2dac07d64a7f6c0bd97d5a5a974847269f99455 registry.digitalocean.com

.PHONY: gcp-login
gcp-login:
	@cat $(PROJECT_ROOT)/gcp_dev_image_push.json | docker login -u _json_key --password-stdin https://gcr.io
