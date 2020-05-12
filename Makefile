# Scripts to handle radish build and installation
# Shell to use with Make
SHELL := /bin/bash

# Build Environment
PACKAGE = radish
PBPKG = $(CURDIR)/api

# Commands
GOCMD = go
GODOC = godoc
PROTOC = protoc
GORUN = $(GOCMD) run
GOGET = $(GOCMD) get
GOTEST = $(GOCMD) test
GOINSTALL = $(GOCMD) install
GOCLEAN = $(GOCMD) clean
GOGENERATE = $(GOCMD) generate

# Output Helpers
BM  = $(shell printf "\033[34;1m●\033[0m")
GM = $(shell printf "\033[32;1m●\033[0m")
RM = $(shell printf "\033[31;1m●\033[0m")


# Export targets not associated with files.
.PHONY: all install radish turnip test citest clean doc protobuf

# Ensure dependencies are installed, run tests and compile
all: install test

# Build the various binaries and sources
install: generate radish

# Build and install the radish command in the $GOBIN directory
radish:
	$(info $(GM) compiling radish executable with go install …)
	@ $(GOINSTALL) ./cmd/radish

# Build and install the turnip command in the $GOBIN directory
turnip:
	$(info $(GM) compiling turnip executable with go install …)
	@ $(GOINSTALL) ./cmd/turnip

# Run go generate to build protocol buffers and other files
generate:
	$(info $(BM) running go generate …)
	@ $(GOGENERATE) ./...

# Target for simple testing on the command line
test:
	$(info $(BM) running simple local tests …)
	@ $(GOTEST) -v ./...

# Target for testing in continuous integration
citest:
	$(info $(BM) running CI tests with randomization and race …)
	$(GOTEST) -bench=. -v --cover --race ./...

# Run Godoc server and open browser to the documentation
doc:
	$(info $(BM) running go documentation server at http://localhost:6060)
	$(info $(BM) type CTRL+C to exit the server)
	@ open http://localhost:6060/pkg/github.com/kansaslabs/radish/
	@ $(GODOC) --http=:6060

# Clean build files
clean:
	$(info $(RM) cleaning up build …)
	@ $(GOCLEAN)
	@ find . -name "*.coverprofile" -print0 | xargs -0 rm -rf

# Compile protocol buffers (use go generate instead)
protobuf:
	$(info $(GM) compiling protocol buffers …)
	@ $(PROTOC) -I $(PBPKG) $(PBPKG)/*.proto --go_out=plugins=grpc:$(PBPKG)