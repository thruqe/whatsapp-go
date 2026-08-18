ifeq ($(OS),Windows_NT)
	BINARY_EXT := .exe
else
	UNAME_S := $(shell uname -s 2>/dev/null)
	ifneq (,$(findstring MINGW,$(UNAME_S)))
		BINARY_EXT := .exe
	else ifneq (,$(findstring MSYS,$(UNAME_S)))
		BINARY_EXT := .exe
	else ifneq (,$(findstring CYGWIN,$(UNAME_S)))
		BINARY_EXT := .exe
	else
		BINARY_EXT :=
	endif
endif

BINARY_NAME := whatsrook$(BINARY_EXT)
BIN_PATH := bin/$(BINARY_NAME)

.PHONY: install fmt test update build clean help

.DEFAULT_GOAL := help

%:
	@:

help:
	@echo Available targets:
	@echo   install   Install Go module dependencies
	@echo   fmt       Format and vet all Go code
	@echo   test      Run the test suite
	@echo   update    Upgrade all Go dependencies
	@echo   build     Build binary executable into $(BIN_PATH)
	@echo   clean     Remove build artifacts and temporary files

install:
	go mod download
	go mod tidy
	cd cli && go mod download && go mod tidy

fmt:
	go fmt ./... && gofmt -w -s .
	cd cli && go fmt ./... && gofmt -w -s .
	go vet ./...
	cd cli && go vet ./...

test:
	@PKGS=$$(go list -f '{{if .TestGoFiles}}{{.ImportPath}}{{end}}' ./...) && \
	  if [ -n "$$PKGS" ]; then go test -v -timeout 120s $$PKGS; fi
	cd wa-core && go test -v -timeout 60s ./...
	cd cli && go test -v -timeout 60s ./...

update:
	go get -u ./...
	go mod tidy
	cd cli && go get -u ./... && go mod tidy

build:
ifeq ($(OS),Windows_NT)
	@if not exist bin mkdir bin
else
	@mkdir -p bin
endif
	cd cli && go build -v -o ../bin/$(BINARY_NAME) .

clean:
ifeq ($(OS),Windows_NT)
	@if exist bin rmdir /s /q bin
	@if exist tmp rmdir /s /q tmp
else
	rm -rf bin/ tmp/
endif