# ==============================================================================
# Multi-Stage Dockerfile for YouTube Data Harvester
# Final image size: ~20 MB (scratch runtime)
# ==============================================================================

# Stage 1: Build static binary
FROM golang:1.27-alpine AS builder

RUN apk add --no-cache ca-certificates git

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /harvester ./cmd/harvester

# Stage 2: Minimal runtime
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /harvester /harvester
COPY --from=builder /app/configs /configs

VOLUME ["/data"]

ENTRYPOINT ["/harvester", "-config", "/configs/harvester.yaml"]
