SERVER_DIR := server
BOTRUNNER_DIR := botrunner
TIMER_DIR := timer

.PHONY: compile-proto
compile-proto:
	$(MAKE) -C $(SERVER_DIR) compile-proto
	$(MAKE) -C $(BOTRUNNER_DIR) compile-proto

.PHONY: build
build:
	$(MAKE) -C $(SERVER_DIR) build
	$(MAKE) -C $(BOTRUNNER_DIR) build
	$(MAKE) -C $(TIMER_DIR) build

.PHONY: docker-build
docker-build:
	$(MAKE) -C $(SERVER_DIR) docker-build
	$(MAKE) -C $(BOTRUNNER_DIR) docker-build
	$(MAKE) -C $(TIMER_DIR) docker-build

.PHONY: publish
publish:
	$(MAKE) -C $(SERVER_DIR) publish
	$(MAKE) -C $(BOTRUNNER_DIR) publish
	$(MAKE) -C $(TIMER_DIR) publish

.PHONY: test
test:
	$(MAKE) -C gamescript test
	$(MAKE) -C encryption test
	$(MAKE) -C server test

.PHONY: system-test
system-test:
	$(MAKE) -C $(BOTRUNNER_DIR) system-test-build
	$(MAKE) -C $(BOTRUNNER_DIR) system-test

# Delegate the other targets to the game server Makefile for now.
.PHONY: %
%:
	$(MAKE) -C $(SERVER_DIR) $@
