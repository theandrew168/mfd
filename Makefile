.POSIX:
.SUFFIXES:

.PHONY: default
default: build

.PHONY: build
build:
	go build -o mfd main.go

.PHONY: run
run:
	go run main.go

.PHONY: test
test:
	go test -count=1 -shuffle=on -race -vet=all -failfast ./...

.PHONY: cover
cover:
	go test -coverprofile=c.out -coverpkg=./... -count=1 ./...
	go tool cover -html=c.out

.PHONY: release
release:
	goreleaser release --clean --snapshot

.PHONY: format
format:
	gofmt -l -s -w .

.PHONY: update
update: update-deps

.PHONY: update-deps
update-deps:
	go get -u ./...
	go mod tidy

.PHONY: clean
clean:
	rm -fr mfd c.out dist/ mfd_* active
