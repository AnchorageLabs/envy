# ENVY — minimal build/run targets.
#
# This iteration intentionally ships no CI/unit tests. These targets cover the
# build + local run loop and a throwaway Postgres for the API.
SHELL := /bin/bash

# A fresh 32-byte AES-256 master key (base64). Used only if the env file below
# does not already define ENVY_MASTER_KEY_B64. Override: make run-api MASTER_KEY=...
MASTER_KEY  ?= $(shell head -c 32 /dev/urandom | base64)
ENVY_DB_URL ?= postgres://envy:envy@localhost:5432/envy?sslmode=disable
ENVY_ADDR   ?= :8080

# Env file run-api loads ENVY_DB_URL / ENVY_MASTER_KEY_B64 from. Defaults to a
# repo-local .env; point it elsewhere to reuse another project's, e.g.:
#   make run-api ENV_FILE=../anchorage-orchestrator/.env
ENV_FILE    ?= .env

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

db: ## build + run the ENVY Postgres image (docker/postgres/Dockerfile)
	docker build -t envy-postgres docker/postgres
	docker run --rm -d --name envy-pg -p 5432:5432 envy-postgres

db-stop: ## stop the ENVY Postgres
	docker stop envy-pg

run-api: api ## migrate + serve, loading ENVY_* from $(ENV_FILE)
	@test -f '$(ENV_FILE)' || { echo "env file not found: $(ENV_FILE) (override with ENV_FILE=...)"; exit 1; }
	set -a; . '$(ENV_FILE)'; set +a; \
	  ENVY_MASTER_KEY_B64="$${ENVY_MASTER_KEY_B64:-$(MASTER_KEY)}" \
	  ENVY_DB_URL="$${ENVY_DB_URL:-$(ENVY_DB_URL)}" \
	  ENVY_ADDR="$${ENVY_ADDR:-$(ENVY_ADDR)}" \
	  ./bin/envyd

clean: ## remove build output
	rm -rf bin
