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
go install google.golang.org/protobuf/cmd/protoc-gen-go
or this
go get -u github.com/golang/protobuf/protoc-gen-go

You may need to update the path, if you get the following error:
protoc-gen-go: program not found or is not executable
--go_out: protoc-gen-go: Plugin failed with status code 1.

export PATH=$PATH:$HOME/go/bin


# Run nats server
We will use NATS messaging server as a broker between game server
and player clients. To run nats server in the dev environment, 
use the following command to build and run nats server.

make run-nats

