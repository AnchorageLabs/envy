# ENVY — minimal build/run targets.
#
# This iteration intentionally ships no CI/unit tests. These targets cover the
# build + local run loop and a throwaway Postgres for the API.
SHELL := /bin/bash

# A fresh 32-byte AES-256 master key (base64). Override to keep secrets stable
# across restarts: make run-api MASTER_KEY=...
MASTER_KEY  ?= $(shell head -c 32 /dev/urandom | base64)
ENVY_DB_URL ?= postgres://envy:envy@localhost:5432/envy?sslmode=disable
ENVY_ADDR   ?= :8080

.PHONY: help tidy build cli api vet db db-stop run-api clean
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  %-12s %s\n", $$1, $$2}'

tidy: ## populate go.sum for both modules
	cd cli && go mod tidy
	cd api && go mod tidy

cli: tidy ## build the envy CLI -> bin/envy
	cd cli && go build -o ../bin/envy ./cmd/envy

api: tidy ## build the envyd API -> bin/envyd
	cd api && go build -o ../bin/envyd ./cmd/envyd

build: cli api ## build both binaries

vet: ## go vet both modules
	cd cli && go vet ./...
	cd api && go vet ./...

db: ## start a throwaway Postgres (needs Docker; sudo in this environment)
	docker run --rm -d --name envy-pg \
	  -e POSTGRES_USER=envy -e POSTGRES_PASSWORD=envy -e POSTGRES_DB=envy \
	  -p 5432:5432 postgres:16

db-stop: ## stop the throwaway Postgres
	docker stop envy-pg

run-api: api ## migrate + serve the API on $(ENVY_ADDR)
	ENVY_MASTER_KEY_B64='$(MASTER_KEY)' ENVY_DB_URL='$(ENVY_DB_URL)' ENVY_ADDR='$(ENVY_ADDR)' ./bin/envyd

clean: ## remove build output
	rm -rf bin
