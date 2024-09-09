all: build usagemd
	@:

.PHONY: build
build:
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w" --trimpath -o bin/sshpuppet-linux-amd64 .
	@GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w" --trimpath -o bin/sshpuppet-windows-amd64 .

usagemd:
	@go run . --help 2>&1 > ./USAGE.md
