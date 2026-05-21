# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o main ./cmd/api

# Final stage
FROM alpine:3.22

RUN apk add --no-cache ca-certificates \
    && addgroup -S app \
    && adduser -S -G app app

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/main .

USER app

# Expose port
EXPOSE 8080

# API and worker roles share this image, so health checks belong to each service definition.

# Command to run the application
CMD ["./main"]
