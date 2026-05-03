BINARY     := forensic-agent
IMAGE      := forensic-agent:latest
BUILD_DIR  := bin

.PHONY: build test lint docker-build docker-push clean

build:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -trimpath -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY) ./cmd/agent

test:
	go test ./... -v -race

lint:
	go vet ./...

docker-build:
	docker build -t $(IMAGE) .

docker-push:
	docker push $(IMAGE)

# Deploy to Kubernetes (creates namespace, RBAC, DaemonSet)
deploy:
	kubectl apply -f deployments/rbac.yaml
	kubectl apply -f deployments/daemonset.yaml

undeploy:
	kubectl delete -f deployments/daemonset.yaml --ignore-not-found
	kubectl delete -f deployments/rbac.yaml --ignore-not-found

# Start local test environment
up:
	docker compose -f deployments/docker-compose.yaml up -d

down:
	docker compose -f deployments/docker-compose.yaml down

clean:
	rm -rf $(BUILD_DIR)
