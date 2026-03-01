.PHONY: build test lint clean docker-build dev-cluster dev-deploy test-integration backtest

BINARY_DIR := bin

build:
	go build -o $(BINARY_DIR)/sage-agent ./cmd/agent
	go build -o $(BINARY_DIR)/sage-server ./cmd/server
	go build -o $(BINARY_DIR)/sage-cli ./cmd/cli

test:
	go test ./... -v -race -count=1

lint:
	go vet ./...
	@which staticcheck > /dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed, skipping"

clean:
	rm -rf $(BINARY_DIR)

docker-build:
	docker build -f Dockerfile.agent -t k8s-sage-agent:latest .
	docker build -f Dockerfile.server -t k8s-sage-server:latest .

dev-cluster:
	./scripts/kind-cluster.sh

dev-deploy:
	helm upgrade --install k8s-sage deploy/helm/k8s-sage

backtest:
	go test ./test/backtest/... -v -count=1

test-integration:
	go test ./test/integration/... -v -tags=integration
