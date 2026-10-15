.PHONY: all backend frontend test test-backend test-postgres lint run run-sim docker up down clean

all: backend frontend

backend:
	cd backend && GOTOOLCHAIN=local go build -o bin/fleetd ./cmd/fleetd

frontend:
	cd frontend && npm ci && npm run build

test: test-backend
	cd frontend && npm run lint

test-backend:
	cd backend && GOTOOLCHAIN=local go vet ./... && go test ./... -timeout 600s

# Nécessite une base PostgreSQL : FM_TEST_DATABASE_URL=postgres://user:pw@host:5432/db?sslmode=disable
test-postgres:
	cd backend && go test ./internal/store/ -run TestPostgresStore -v

lint:
	cd backend && gofmt -l . && go vet ./...
	cd frontend && npm run lint

# Lance le backend seul (store mémoire + broker MQTT embarqué) sur :8080 / :1883
run: backend
	cd backend && FM_ADMIN_PASSWORD=admin ./bin/fleetd

# Idem avec 8 AGV simulés et génération automatique de missions
run-sim: backend
	cd backend && FM_ADMIN_PASSWORD=admin FM_SIM_AUTOSTART=8 FM_SIM_TIMESCALE=2 ./bin/fleetd

docker:
	docker compose -f deploy/docker-compose.yml build

up:
	docker compose -f deploy/docker-compose.yml up -d

down:
	docker compose -f deploy/docker-compose.yml down

clean:
	rm -rf backend/bin frontend/.next
