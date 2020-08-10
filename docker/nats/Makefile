.PHONY:
build:
	docker build -f Dockerfile.nats -t nats-server .

.PHONY:
run:
	docker run -it -p 4222:4222 -p 8222:8222 -p 9222:9222 nats-server:latest
