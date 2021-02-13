# All targets delegate to the gameserver (server) for now.

SERVER_DIR := server

.PHONY: %
%:
	$(MAKE) -C $(SERVER_DIR) $@
