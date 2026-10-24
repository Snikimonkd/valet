LOCAL_BIN:=$(CURDIR)/bin
PG_DSN:=postgres://postgres@localhost:5432/valet?sslmode=disable
DOMAIN:=localhost

run:
	PG_DSN="$(PG_DSN)" DOMAIN=$(DOMAIN) go run ./cmd

# локальная база для разработки и тестов: postgres 17 без пароля, база valet
postgres-up:
	docker run -d --name valet-postgres -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=valet -p 5432:5432 postgres:17

test:
	PG_DSN="$(PG_DSN)" go test -race -cover -coverprofile=cover.out.tmp -covermode=atomic -coverpkg ./... ./...

test-cov:test
	cat cover.out.tmp | grep -v "mock.go" | grep -v "/testsupport/" | grep -v "/generated/" > cover.out || cp cover.out.tmp cover.out
	go tool cover -func=cover.out

bin-deps:
	GOPROXY="proxy.golang.org" GOBIN=$(LOCAL_BIN) go install github.com/go-jet/jet/v2/cmd/jet@v2.16.1
	GOPROXY="proxy.golang.org" GOBIN=$(LOCAL_BIN) go install github.com/pressly/goose/v3/cmd/goose@v3.29.0
	GOPROXY="proxy.golang.org" GOBIN=$(LOCAL_BIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0

# модели и построители запросов из схемы базы: после каждой миграции
jet:
	@$(LOCAL_BIN)/jet -dsn "$(PG_DSN)" -path=./internal/generated -schema=public -ignore-tables=goose_db_version

mocks:
	go generate ./...

lint:
	@$(LOCAL_BIN)/golangci-lint run --config=.golangci.yml

migrate-up:
	@$(LOCAL_BIN)/goose -dir migrations postgres "$(PG_DSN)" up

migrate-down:
	$(LOCAL_BIN)/goose -dir migrations postgres "$(PG_DSN)" down

create-migration:
	$(LOCAL_BIN)/goose -dir migrations create $(NAME) sql

.PHONY: run postgres-up test test-cov bin-deps jet mocks lint migrate-up migrate-down create-migration
