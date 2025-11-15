# Build stage
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o p2p-sync ./cmd/p2p-sync

# Runtime stage
FROM ubuntu:24.04
RUN apt-get update && apt-get install -y \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/p2p-sync /usr/local/bin/p2p-sync

EXPOSE 8080 8081
CMD ["p2p-sync"]

