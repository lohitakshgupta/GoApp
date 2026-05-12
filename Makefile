.PHONY: run test race vet fmt docker-build

run:
	go run ./cmd/server

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

docker-build:
	docker build -t job-runner-service:local .
