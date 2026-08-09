SHELL := /bin/bash
.DEFAULT_GOAL := help

BIN     := bin/hub
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
COMPOSE := docker compose -f deployments/docker-compose.yml --env-file .env

.PHONY: help
help: ## List available targets
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

# ------------------------------------------------------------------------------
# Setup
# ------------------------------------------------------------------------------

.PHONY: setup
setup: .env secrets hooks ## Prepare the local environment from scratch
	@echo "✔ environment ready. Next: make up"

.env:
	@cp .env.example .env
	@echo "✔ .env created from the template — run 'make secrets' to generate credentials"

.PHONY: secrets
secrets: ## Generate random secrets in .env and the broker password file
	@test -f .env || (echo "✘ .env not found. Run 'make setup'." && exit 1)
	@PGPASS=$$(openssl rand -base64 24 | tr -d '/+=' | head -c 32); \
	 MQPASS=$$(openssl rand -base64 24 | tr -d '/+=' | head -c 32); \
	 HOMEID=$$(cat /proc/sys/kernel/random/uuid); \
	 sed -i "s|^POSTGRES_PASSWORD=.*|POSTGRES_PASSWORD=$$PGPASS|" .env; \
	 sed -i "s|^HUB_DB_DSN=.*|HUB_DB_DSN=postgres://smarthome:$$PGPASS@localhost:5432/smarthome?sslmode=disable|" .env; \
	 sed -i "s|^HUB_MQTT_PASSWORD=.*|HUB_MQTT_PASSWORD=$$MQPASS|" .env; \
	 sed -i "s|^MQTT_HUB_PASSWORD=.*|MQTT_HUB_PASSWORD=$$MQPASS|" .env; \
	 sed -i "s|^HUB_HOME_ID=.*|HUB_HOME_ID=$$HOMEID|" .env
	@$(MAKE) --no-print-directory mosquitto-passwd
	@echo "✔ secrets generated in .env (git-ignored)"

.PHONY: mosquitto-passwd
mosquitto-passwd: ## Generate deployments/mosquitto/passwd from .env
	@# The file is created, chowned and chmodded inside the container, because
	@# mosquitto runs as uid 1883 and refuses (rightly) to read a password file
	@# it does not own. End result: mode 0600, owned by the broker user — not
	@# even the host user can read it.
	@set -a; source .env; set +a; \
	 docker run --rm \
	   -e MQTT_HUB_USERNAME -e MQTT_HUB_PASSWORD \
	   -v "$(PWD)/deployments/mosquitto:/cfg" \
	   --entrypoint sh eclipse-mosquitto:2 -c \
	   ': > /cfg/passwd \
	    && mosquitto_passwd -b /cfg/passwd "$$MQTT_HUB_USERNAME" "$$MQTT_HUB_PASSWORD" \
	    && chown 1883:1883 /cfg/passwd \
	    && chmod 0600 /cfg/passwd'
	@echo "✔ deployments/mosquitto/passwd generated (git-ignored, owned by broker uid)"

.PHONY: hooks
hooks: ## Install git hooks (gitleaks on pre-commit)
	@git config core.hooksPath .githooks
	@chmod +x .githooks/*
	@echo "✔ hooks installed (core.hooksPath = .githooks)"

# ------------------------------------------------------------------------------
# Environment
# ------------------------------------------------------------------------------

.PHONY: up
up: ## Start broker and database, waiting until healthy
	$(COMPOSE) up -d --wait
	@echo "✔ dependencies healthy"

.PHONY: down
down: ## Stop the containers (volumes preserved)
	$(COMPOSE) down

.PHONY: clean
clean: ## Stop containers AND DELETE data volumes
	$(COMPOSE) down -v
	@rm -rf bin/

.PHONY: logs
logs: ## Follow container logs
	$(COMPOSE) logs -f

# ------------------------------------------------------------------------------
# Application
# ------------------------------------------------------------------------------

.PHONY: build
build: ## Build the hub binary
	@mkdir -p bin
	go build $(LDFLAGS) -o $(BIN) ./cmd/hub

.PHONY: run
run: ## Run the hub locally with .env loaded
	@set -a; source .env; set +a; go run $(LDFLAGS) ./cmd/hub

.PHONY: test
test: ## Run tests with the race detector
	go test -race -cover ./...

.PHONY: lint
lint: ## Formatting and static analysis
	go vet ./...
	@test -z "$$(gofmt -l . | tee /dev/stderr)" || (echo "✘ unformatted files"; exit 1)

.PHONY: tidy
tidy: ## Tidy module dependencies
	go mod tidy

# ------------------------------------------------------------------------------
# Security
# ------------------------------------------------------------------------------

.PHONY: sec
sec: ## Scan for secrets and known vulnerabilities
	@command -v gitleaks >/dev/null || (echo "✘ gitleaks missing: go install github.com/zricethezav/gitleaks/v8@latest"; exit 1)
	gitleaks detect --no-banner --redact
	@command -v govulncheck >/dev/null || (echo "✘ govulncheck missing: go install golang.org/x/vuln/cmd/govulncheck@latest"; exit 1)
	govulncheck ./...

# ------------------------------------------------------------------------------
# SH-001 verification
# ------------------------------------------------------------------------------

.PHONY: smoke
smoke: ## Verify the SH-001 acceptance criteria
	@set -a; source .env; set +a; \
	 echo "→ publishing test telemetry to the broker..."; \
	 docker run --rm --network host eclipse-mosquitto:2 mosquitto_pub \
	   -h 127.0.0.1 -p 1883 -u "$$MQTT_HUB_USERNAME" -P "$$MQTT_HUB_PASSWORD" \
	   -t "home/$$HUB_HOME_ID/dev/sim-001/telemetry" \
	   -m '{"soil_moisture":27.4,"ts":"2026-01-01T00:00:00Z"}' \
	 && echo "✔ published — look for the 'MQTT message received' line in the hub log"
	@echo "→ querying /health..."
	@curl -fsS http://localhost:8080/health | (command -v jq >/dev/null && jq . || cat)
