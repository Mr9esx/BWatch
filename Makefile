.PHONY: install install-ytdlp build run dev-server dev-web test

NPM ?= npm

install:
	$(NPM) --prefix web ci --no-audit --no-fund

install-ytdlp:
	./scripts/install-ytdlp.sh

build:
	$(NPM) --prefix web run build
	go build -trimpath -o bin/bilifetch ./cmd/bilifetch

run:
	PATH="$(CURDIR)/.tools/venv/bin:$$PATH" ./bin/bilifetch

dev-server:
	PATH="$(CURDIR)/.tools/venv/bin:$$PATH" BILIFETCH_DEV_ORIGIN=http://127.0.0.1:5173 go run ./cmd/bilifetch

dev-web:
	$(NPM) --prefix web run dev

test:
	go test -race ./...
	go vet ./...
	$(NPM) --prefix web run build
