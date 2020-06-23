# server

# protobuf

### install protobuf compiler
http://google.github.io/proto-lens/installing-protoc.html

### install go generator
go install google.golang.org/protobuf/cmd/protoc-gen-go
go get -u github.com/golang/protobuf/protoc-gen-go

You may need to update the path, if you get the following error:
protoc-gen-go: program not found or is not executable
--go_out: protoc-gen-go: Plugin failed with status code 1.

export PATH=$PATH:$HOME/go/bin
