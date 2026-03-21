# Stage 1: Build the binary​
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum and download dependencies​
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code​
COPY . .

# Build the application​
RUN CGO_ENABLED=0 GOOS=linux go build -o main ./cmd/api/main.go

# Stage 2: Final lightweight image​
FROM alpine:latest

WORKDIR /root/

# Copy the binary from the builder stage​
COPY --from=builder /app/main .
# Copy the .env.example as a template (optional)​
COPY --from=builder /app/.env.example ./.env

EXPOSE 8080

CMD "./main"