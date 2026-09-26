# Multi-stage Docker build for Unified Core Engine
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Copy shared internal module (referenced via go.mod replace directive)
COPY packages/goplatform ./packages/goplatform

# Copy go module files
COPY apps/core-engine/go.mod ./apps/core-engine/
WORKDIR /app/apps/core-engine
RUN go mod download

# Copy source code
WORKDIR /app
COPY apps/core-engine ./apps/core-engine

# Build static binary
WORKDIR /app/apps/core-engine
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/core-engine ./cmd/server

# Final lightweight runtime image
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /bin/core-engine /app/core-engine

ENV PORT=8082
EXPOSE 8082

CMD ["/app/core-engine"]
