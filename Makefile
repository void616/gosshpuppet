BUILD_VERSION=$(shell git tag --points-at HEAD --sort=-refname | head -1)
BUILD_LD_FLAGS=-ldflags '-s -w -X "gosshpuppet/internal.Version=$(BUILD_VERSION)"'

all: helpmd build
	@:

build:
	@if [ -z "$(BUILD_VERSION)" ]; then echo "No version tag found"; exit 1; fi
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_LD_FLAGS) --trimpath -o bin/sshpuppet-linux-amd64 .
	@GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_LD_FLAGS) --trimpath -o bin/sshpuppet-windows-amd64 .

helpmd:
	@echo '```text' > ./HELP.md
	@go run . --help 2>&1 >> ./HELP.md
	@echo '```' >> ./HELP.md
