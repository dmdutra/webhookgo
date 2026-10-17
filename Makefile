.PHONY: up down logs tunnel-url build web test check dev-redis dev-server dev-web clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

## Docker ----------------------------------------------------------------------
up:            ## Build and start the stack (http://localhost:8080)
	docker compose up -d --build

down:          ## Stop the stack (data is kept in the redis-data volume)
	docker compose down

logs:          ## Follow the app logs
	docker compose logs -f app

tunnel-url:    ## Print the Cloudflare Quick Tunnel URL (deploy/cloudflare/quick.yml)
	@url=$$(docker compose logs cloudflared 2>/dev/null | grep -o 'https://[a-z0-9-]*\.trycloudflare\.com' | tail -1); \
	if [ -n "$$url" ]; then echo "$$url"; else echo "No Quick Tunnel URL yet (is COMPOSE_FILE set to deploy/cloudflare/quick.yml in .env?)"; exit 1; fi

## Local build -----------------------------------------------------------------
web:           ## Build the web UI into web/dist
	cd web && npm ci && npm run build

build: web     ## Build bin/webhookgo with the UI embedded
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o bin/webhookgo ./cmd/webhookgo

test:          ## Go tests (race detector) + Svelte type check
	go test -race ./...
	cd web && npm run check

## Development (three terminals) ---------------------------------------------
dev-redis:     ## Redis on :6379
	docker run --rm -p 6379:6379 redis:8-alpine

dev-server:    ## Go API on :8080
	go run ./cmd/webhookgo

dev-web:       ## Vite dev server with hot reload on :5173 (proxies the API to :8080)
	cd web && npm install && npm run dev

clean:
	rm -rf bin web/dist/assets web/dist/index.html web/dist/*.svg web/dist/*.js

help:
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | sed 's/:.*##/\t/'
