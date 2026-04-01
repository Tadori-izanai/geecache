# Go parameters
GOCMD=GO111MODULE=on go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test

all: test build
build:
	rm -rf target/
	mkdir target/
	$(GOBUILD) -o target/http-test cmd/http-test/main.go
	$(GOBUILD) -o target/http-node cmd/http-nodes/main.go
	$(GOBUILD) -o target/grpc-node cmd/grpc-nodes/main.go

test:
	$(GOTEST) -v ./...

clean:
	rm -rf target/

run-http:
	nohup target/http-node -port=8001 2>&1 > target/8001.log &
	nohup target/http-node -port=8002 2>&1 > target/8002.log &
	nohup target/http-node -port=8003 -api=true 2>&1 > target/8003.log &

run:
	nohup target/grpc-node -port=8001 2>&1 > target/8001.log &
	nohup target/grpc-node -port=8002 2>&1 > target/8002.log &
	nohup target/grpc-node -port=8003 -api=true 2>&1 > target/8003.log &

stop-http:
	pkill -f target/http-node

stop:
	pkill -f target/grpc-node
