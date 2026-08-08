# Multi-stage Dockerfile for GoBalancer POC
FROM docker.io/library/golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/gobalancer ./cmd/gobalancer

# Final minimal runtime image
FROM docker.io/library/alpine:latest

WORKDIR /app

RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /app/gobalancer /app/gobalancer
COPY --from=builder /app/configs/config.yaml /app/configs/config.yaml

EXPOSE 8080

ENTRYPOINT ["/app/gobalancer", "configs/config.yaml"]
