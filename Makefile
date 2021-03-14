SERVER_DIR := server
BOTRUNNER_DIR := botrunner
SYSTEM_TEST_DIR := system_test

.PHONY: compile-proto
compile-proto:
	$(MAKE) -C $(SERVER_DIR) compile-proto
	$(MAKE) -C $(BOTRUNNER_DIR) compile-proto

.PHONY: build
build:
	$(MAKE) -C $(SERVER_DIR) build
	$(MAKE) -C $(BOTRUNNER_DIR) build
	$(MAKE) -C $(SYSTEM_TEST_DIR) build

.PHONY: test
test:
	$(MAKE) -C gamescript test
	$(MAKE) -C server test

.PHONY: system_test
system_test:
	$(MAKE) -C $(SYSTEM_TEST_DIR) docker-test

# Delegate the other targets to the game server Makefile for now.
.PHONY: %
%:
	$(MAKE) -C $(SERVER_DIR) $@
