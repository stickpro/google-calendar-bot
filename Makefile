.PHONY:
.SILENT:
.DEFAULT_GOAL := run

build:
	go mod download && CGO_ENABLED=1 GOOS=linux go build -o ./.bin/event-bot ./cmd/app/main.go && cp ./config.yml ./.bin/config.yml

run: build
	docker-compose up --remove-orphans event-bot

rebuild:
	docker-compose up -d --no-deps --build