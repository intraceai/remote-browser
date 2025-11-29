FROM golang:1.22-bookworm AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o agent ./cmd/agent

FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive

# Install Chromium and minimal dependencies
RUN apt-get update && apt-get install -y \
    chromium \
    fonts-liberation \
    fonts-noto-color-emoji \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/agent .

# Chromium path on Debian
ENV CHROME_PATH=/usr/bin/chromium

EXPOSE 8082

CMD ["./agent"]
