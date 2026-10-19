.PHONY: install build run dev-server dev-web test

NPM ?= npm

install:
	$(NPM) --prefix web ci --no-audit --no-fund

build:
	$(NPM) --prefix web run build
	go build -trimpath -o bin/bilifetch ./cmd/bilifetch

run:
	./bin/bilifetch

dev-server:
	BILIFETCH_DEV_ORIGIN=http://127.0.0.1:5173 go run ./cmd/bilifetch

dev-web:
	$(NPM) --prefix web run dev

test:
	go test -race ./...
	go vet ./...
	$(NPM) --prefix web run build
