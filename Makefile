# Go parameters
GOCMD=GO111MODULE=on go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test

all: test build
build:
	rm -rf target/
	mkdir target/
	$(GOBUILD) -o target/http-server cmd/http-server/main.go
	$(GOBUILD) -o target/server cmd/multi-nodes/main.go

test:
	$(GOTEST) -v ./...

clean:
	rm -rf target/

run:
	nohup target/server -port=8001 2>&1 > target/8001.log &
	nohup target/server -port=8002 2>&1 > target/8002.log &
	nohup target/server -port=8003 -api=true 2>&1 > target/8003.log &

stop:
	pkill -f target/server
