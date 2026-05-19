BINARY     := forensic-agent
IMAGE      := forensic-agent:latest
BUILD_DIR  := bin

.PHONY: build test lint docker-build docker-push clean up down falco falco-install

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

# Start local test environment (forensic-agent + target container)
# Run 'make falco' in a separate terminal to start Falco on the host.
up:
	sudo mkdir -p /var/forensics
	docker compose -f deployments/docker-compose.yaml up -d --build

down:
	docker compose -f deployments/docker-compose.yaml down

# Install Falco directly on the host (Ubuntu/Debian). Run once after cloning.
falco-install:
	curl -fsSL https://falco.org/repo/falcosecurity-packages.asc | \
		sudo gpg --dearmor -o /usr/share/keyrings/falco-archive-keyring.gpg
	echo "deb [signed-by=/usr/share/keyrings/falco-archive-keyring.gpg] \
https://download.falco.org/packages/deb stable main" | \
		sudo tee /etc/apt/sources.list.d/falcosecurity.list
	sudo apt-get update -y
	FALCO_DRIVER_CHOICE=modern_ebpf sudo apt-get install -y falco
	@echo ""
	@echo "Falco installed. Run 'make falco' to start monitoring."

# Start Falco on the host with the project rules and config.
# Run this in a separate terminal alongside 'make up'.
falco:
	sudo falco \
		-c $(CURDIR)/deployments/falco.yaml \
		-r /etc/falco/falco_rules.yaml \
		-r $(CURDIR)/configs/falco-rules.yaml

clean:
	rm -rf $(BUILD_DIR)
