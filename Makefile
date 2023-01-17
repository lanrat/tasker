
test:
	CGO_ENABLED=0 go test -v ./...

update-deps: go.mod
	GOPROXY=direct go get -u ./...
	go mod tidy

deps: go.mod
	go mod download

fmt:
	gofmt -s -w -l .

lint: | lint-staticcheck lint-golangci

lint-staticcheck:
	staticcheck -f stylish -checks all ./...

lint-golangci:
	golangci-lint run