# Go parameters
GOCMD=GO111MODULE=on go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test

all: test build
build:
	rm -rf target/
	mkdir target/
	$(GOBUILD) -o target/http-server cmd/http-server/main.go

test:
	$(GOTEST) -v ./...

clean:
	rm -rf target/

run:
	target/http-server
