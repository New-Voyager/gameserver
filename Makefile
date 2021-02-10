SERVER_DIR := server

.PHONY: %
%:
	$(MAKE) -C $(SERVER_DIR) $@
