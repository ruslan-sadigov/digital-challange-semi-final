DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/smartcity?sslmode=disable
TEST_DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/smartcity_test?sslmode=disable

.PHONY: run demo test test-integration cover db

db:            ## start PostgreSQL in Docker
	docker compose up -d db
	docker compose exec db sh -c 'until pg_isready -U postgres; do sleep 1; done; psql -U postgres -tc "SELECT 1 FROM pg_database WHERE datname='"'"'smartcity_test'"'"'" | grep -q 1 || psql -U postgres -c "CREATE DATABASE smartcity_test"'

run:           ## run with spec timings (10-minute anomaly rule)
	DATABASE_URL=$(DATABASE_URL) go run ./cmd/server

demo:          ## run with fast timings so the whole flow shows in a few minutes
	DEMO=true DATABASE_URL=$(DATABASE_URL) go run ./cmd/server

test:          ## unit tests (no database)
	go test ./internal/...

test-integration: ## integration tests (needs PostgreSQL)
	TEST_DATABASE_URL=$(TEST_DATABASE_URL) go test -tags=integration -count=1 -p 1 ./test/integration/...

cover:         ## combined unit + integration coverage
	TEST_DATABASE_URL=$(TEST_DATABASE_URL) go test -tags=integration -count=1 -p 1 -coverpkg=./internal/... -coverprofile=cover.out ./internal/... ./test/integration/...
	go tool cover -func=cover.out | tail -1
