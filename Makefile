.PHONY: run build test test-integration lint sqlc db-up db-down

run: ## запуск локально (polling; env из .env)
	go run ./cmd/bot

build:
	go build -o bin/bot ./cmd/bot

test:
	go test ./...

test-integration: ## интеграционные тесты с реальным Postgres (нужен Docker)
	go test -tags=integration ./internal/storage/...

lint:
	go vet ./...
	golangci-lint run

sqlc: ## перегенерация типобезопасного кода из SQL
	sqlc generate

db-up: ## Postgres для локальной разработки
	docker compose -f deploy/docker-compose.dev.yml up -d

db-down:
	docker compose -f deploy/docker-compose.dev.yml down
