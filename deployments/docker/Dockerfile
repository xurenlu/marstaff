# Multi-stage build for Marstaff

FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-w -s' -o bin/gateway ./cmd/gateway

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary
COPY --from=builder /build/bin/gateway /app/

# Copy configs and skills
COPY --from=builder /build/configs /app/configs
COPY --from=builder /build/skills /app/skills

# Expose port
EXPOSE 18789

# Default command
CMD ["/app/gateway"]
