.PHONY: help build
default: help

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

build: bin build-windows build-linux

bin:
	@mkdir bin || true

build-windows:
	env GOOS=windows GOARCH=amd64 go build -o release/demo-export.exe

build-linux:
	env GOOS=linux GOARCH=amd64 go build -o release/demo-export