default: help

.PHONY: help
help: # Show help for each of the Makefile recipes
	@grep -E '^[a-zA-Z0-9 -]+:.*#' Makefile | sort | while read -r l; do printf "\033[1;32m$$(echo $$l | cut -f 1 -d':')\033[00m:$$(echo $$l | cut -f 2- -d'#')\n"; done

.PHONY: server
server: out/server # Build the server application

out/server: $(wildcard cmd/server/**/*.go) $(wildcard internal/**/*.go)
	go build -o out/server ./cmd/server

.PHONY: lambda
lambda: out/function.zip # Build the AWS lambda application

out/function.zip: $(wildcard cmd/lambda/**/*.go) $(wildcard internal/**/*.go)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags lambda.norpc -o out/bootstrap ./cmd/lambda && cd out && zip function.zip bootstrap

.PHONY: clean
clean: # Delete any build artefacts
	rm -rf out
