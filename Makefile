.PHONY: build build-no-worker test fmt vet lint generate-proto ci

build:
	go build -o vot ./cmd/vot

build-no-worker:
	go build -tags no_worker -o vot ./cmd/vot

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

vet:
	go vet ./...

lint: fmt vet test

generate-proto:
	protoc --go_out=internal/yandexproto --go_opt=paths=source_relative internal/yandexproto/yandex.proto

ci: vet test
