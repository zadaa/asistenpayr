# Stage 1: Build binary
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy dependency manifests
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build lightweight CGO-free static binary for Linux
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o merge-lembur main.go

# Stage 2: Production minimal image
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy compiled binary from builder
COPY --from=builder /app/merge-lembur .

# Default port
ENV PORT=8080
EXPOSE 8080

CMD ["./merge-lembur"]
