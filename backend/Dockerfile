# Build stage
FROM golang:1.26.5-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS builder

WORKDIR /app

# Install dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application and maintenance CLI
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o main ./cmd/api \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o pmsctl ./cmd/pmsctl

# Final stage
FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

RUN apk add --no-cache ca-certificates \
    && addgroup -S app \
    && adduser -S -G app app \
    && mkdir -p /data/attachments \
    && chown app:app /data/attachments

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/main .
COPY --from=builder /app/pmsctl .

USER app

# Expose port
EXPOSE 8080

# API and worker roles share this image, so health checks belong to each service definition.

# Command to run the application
CMD ["./main"]
