# ---- Build stage ----
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/techtransfer-agency   ./cmd/techtransfer-agency

# ---- Runtime stage ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /out/techtransfer-agency   /usr/local/bin/techtransfer-agency

WORKDIR /app
