# gameserver

[![CircleCI](https://circleci.com/gh/New-Voyager/gameserver.svg?style=svg&circle-token=15669e5d94af5df5bde7e4bcbf095dd3b89263bc)](https://app.circleci.com/pipelines/github/New-Voyager/gameserver)

## Testing

Run below make target to run the unit tests.
```
make test
```

### install protobuf compiler
http://google.github.io/proto-lens/installing-protoc.html

### install go generator
```
go install google.golang.org/protobuf/cmd/protoc-gen-go
```
or this
```
go get -u github.com/golang/protobuf/protoc-gen-go
```

You may need to update the path, if you get the following error:
```
protoc-gen-go: program not found or is not executable
--go_out: protoc-gen-go: Plugin failed with status code 1.
```
export PATH=$PATH:$HOME/go/bin


# Build the images
Build the game-server and NATS images using the following command.
```
make docker-build
```

# Run nats server
We will use NATS messaging server as a broker between game server
and player clients. To run nats server in the dev environment, 
use the following command to run nats server.
```
make run-nats
```

# Run Redis server
Run the Redis server. Redis server is used to store the game state.
```
make run-redis
```

# Run game server
Run the game server.
```
make run-server
```

# Run bot testing
Run bot testing.
```
make run-bot
```

# Developer workflow
```
# Build the images.
make docker-build

# Start the infrastructure components.
make run-nats
make run-redis

# Run the game server and the bot.
# Either run them from the shell using below commands
# or run them from VS code for debugging.
make run-server
make run-bot
```
