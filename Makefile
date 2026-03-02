.PHONY: build test lint lint-python lint-helm clean docker-build dev-cluster dev-deploy test-integration backtest test-safety \
	dogfood setup-workloads generate-load validate validate-classifications validate-recommendations validate-savings teardown

BINARY_DIR := bin

build:
	go build -o $(BINARY_DIR)/sage-agent ./cmd/agent
	go build -o $(BINARY_DIR)/sage-server ./cmd/server
	go build -o $(BINARY_DIR)/sage-cli ./cmd/cli

test:
	go test -p 2 -timeout 120s ./... -v -count=1

lint:
	go vet ./...
	@which staticcheck > /dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed, skipping"
	@which ruff > /dev/null 2>&1 && ruff check ml/ || echo "ruff not installed, skipping"
	@which helm > /dev/null 2>&1 && helm lint deploy/helm/k8s-sage/ || echo "helm not installed, skipping"

lint-python:
	ruff check ml/

lint-helm:
	helm lint deploy/helm/k8s-sage/

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

test-safety:
	go test ./test/safety/... -v -count=1

test-integration:
	go test ./test/integration/... -v -tags=integration

dogfood: dev-cluster dev-deploy setup-workloads generate-load

setup-workloads:
	test/dogfood/setup-workloads.sh

generate-load:
	test/dogfood/generate-load.sh

validate: validate-classifications validate-recommendations validate-savings

validate-classifications:
	test/dogfood/validate-classifications.sh

validate-recommendations:
	test/dogfood/validate-recommendations.sh

validate-savings:
	test/dogfood/validate-savings.sh

teardown:
	test/dogfood/teardown.sh
