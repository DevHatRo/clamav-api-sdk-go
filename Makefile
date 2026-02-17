.PHONY: test test-integration lint proto coverage clean

test:
	go test -race -coverprofile=coverage.out ./...
	cd grpc && go test -race -coverprofile=coverage.out ./...

test-integration:
	go test -race -tags=integration -v ./...
	cd grpc && go test -race -tags=integration -v ./...

lint:
	golangci-lint run ./...
	cd grpc && golangci-lint run ./...

proto:
	./scripts/generate-proto.sh

coverage:
	go tool cover -html=coverage.out -o coverage.html

clean:
	rm -f coverage.out coverage.html
	rm -f grpc/coverage.out grpc/coverage.html
