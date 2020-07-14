.PHONY:
build-nats:
	rm -rf nats-server && true
	git clone https://github.com/nats-io/nats-server.git nats-server
	docker build -t nats-builder -f Dockerfile.compile .
	rm -rf nats-server && true

# compiles nats-server for alpine
.PHONY:
compile: build-nats
	docker run  -v "${PWD}":/app nats-builder:latest go build -o /app/nats-server

.PHONY:
build: compile
	docker build -t nats:latest -f Dockerfile.nats .

.PHONY:
run:
	docker run -it -p 4222:4222 -p 9222:9222 nats:latest
