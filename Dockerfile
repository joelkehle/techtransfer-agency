# ---- Build stage ----
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/agent-bus-v2   ./cmd/agent-bus-v2
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/concierge       ./cmd/concierge
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/patent-pipeline ./cmd/patent-pipeline

# ---- Runtime stage ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /out/agent-bus-v2   /usr/local/bin/agent-bus-v2
COPY --from=builder /out/concierge       /usr/local/bin/concierge
COPY --from=builder /out/patent-pipeline /usr/local/bin/patent-pipeline

# Static web assets for the concierge UI
COPY web/ /app/web/

WORKDIR /app
