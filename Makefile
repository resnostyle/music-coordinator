.PHONY: build run test clean init-db

build:
	go build -o music-coordinator .

run: build
	./music-coordinator

test:
	go test -v ./...

clean:
	rm -f music-coordinator
	rm -f *.db *.db-shm *.db-wal

init-db:
	@if [ ! -f music_coordinator.db ]; then \
		echo "Database will be created on first run"; \
	else \
		sqlite3 music_coordinator.db < init_db.sql; \
	fi

deps:
	go mod download
	go mod tidy

docker-build:
	docker build -t music-coordinator .

docker-run:
	docker-compose up -d

docker-stop:
	docker-compose down

