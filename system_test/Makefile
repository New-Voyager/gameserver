DEV_REDIS_HOST := localhost
DEV_REDIS_PORT := 6379
DEV_REDIS_DB := 0
DEV_API_SERVER_URL := http://localhost:9501


.PHONY: build
build:
	go build -o test

.PHONY: run
run: export PERSIST_METHOD=redis
run: export REDIS_HOST=$(DEV_REDIS_HOST)
run: export REDIS_PORT=$(DEV_REDIS_PORT)
run: export REDIS_DB=$(DEV_REDIS_DB)
run: export API_SERVER_URL=$(DEV_API_SERVER_URL)
run:
	./test --config config.yaml --game-server-dir=../server --botrunner-dir=../botrunner
