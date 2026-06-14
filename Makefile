.PHONY: run build tidy test fmt vet docs docker-up docker-down

run:        ## Run the API server
	go run ./cmd/api

docs:       ## Regenerate Swagger docs into ./docs (requires: go install github.com/swaggo/swag/cmd/swag@latest)
	swag init -g cmd/api/main.go -o docs --parseInternal --parseDependency

build:      ## Build the API binary into ./bin
	go build -o bin/cairn ./cmd/api

tidy:       ## Sync go.mod/go.sum
	go mod tidy

test:       ## Run all tests (integration tests skip unless TEST_DATABASE_URL is set)
	go test ./... -count=1

test-db-up: ## Start a throwaway Postgres for integration tests (port 55432)
	docker run -d --rm --name cairn-test-pg -e POSTGRES_USER=cairn -e POSTGRES_PASSWORD=cairn -e POSTGRES_DB=cairn -p 55432:5432 postgres:16-alpine

test-db-down: ## Stop the throwaway test Postgres
	docker rm -f cairn-test-pg

test-integration: ## Run tests against the throwaway Postgres
	TEST_DATABASE_URL=postgres://cairn:cairn@localhost:55432/cairn?sslmode=disable go test ./... -count=1

fmt:        ## Format code
	go fmt ./...

vet:        ## Static checks
	go vet ./...

docker-up:  ## Start local db + api via docker-compose
	docker compose up --build

docker-down: ## Stop and remove the local stack
	docker compose down
