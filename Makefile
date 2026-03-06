.PHONY: build test lint lint-helm clean docker-build dev-cluster dev-deploy docs-dev docs-build

BINARY_DIR := bin

build:
	go build -o $(BINARY_DIR)/gerty-agent ./cmd/agent
	go build -o $(BINARY_DIR)/gerty-cli ./cmd/cli

test:
	go test -p 2 -timeout 120s ./... -v -count=1

lint:
	go vet ./...
	@which staticcheck > /dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed, skipping"
	@which helm > /dev/null 2>&1 && helm lint deploy/helm/gerty/ || echo "helm not installed, skipping"

lint-helm:
	helm lint deploy/helm/gerty/

clean:
	rm -rf $(BINARY_DIR)

docker-build:
	docker build -f Dockerfile.agent -t gerty-agent:latest .

dev-cluster:
	./scripts/kind-cluster.sh

dev-deploy:
	helm upgrade --install gerty deploy/helm/gerty

docs-dev:
	cd docs && npm run dev

docs-build:
	cd docs && npm run build
