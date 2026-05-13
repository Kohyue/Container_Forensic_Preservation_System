# --- Build stage ---
FROM golang:1.23-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /forensic-agent ./cmd/agent

# --- Runtime stage ---
FROM alpine:3.19

RUN apk add --no-cache docker-cli

COPY --from=builder /forensic-agent /usr/local/bin/forensic-agent

RUN mkdir -p /etc/forensic-agent /var/forensics/evidence
COPY configs/config.yaml /etc/forensic-agent/config.yaml

ENTRYPOINT ["/usr/local/bin/forensic-agent"]
CMD ["-config=/etc/forensic-agent/config.yaml"]
