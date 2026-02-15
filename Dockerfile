FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o music-coordinator .

FROM alpine:latest

# Install sqlite and ca-certificates
RUN apk --no-cache add ca-certificates sqlite

WORKDIR /app

# Copy binary and init script
COPY --from=builder /app/music-coordinator .
COPY --from=builder /app/init_db.sql .

# Create directory for database
RUN mkdir -p /data

EXPOSE 8080

# Default to /data for database persistence
ENV DB_PATH=/data/music_coordinator.db

CMD ["./music-coordinator"]

