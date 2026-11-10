# Build context: this repository.
#   docker build -t core-engine .
FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git ca-certificates tzdata
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/core-engine ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /bin/core-engine /app/core-engine
# WCPA rules for the colour engine (internal/colour); read at startup.
COPY config/wcpa.json /app/config/wcpa.json
ENV WCPA_CONFIG_PATH=/app/config/wcpa.json
ENV PORT=8082
EXPOSE 8082
CMD ["/app/core-engine"]
