# ---- Build stage ----
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/techtransfer-agency   ./cmd/techtransfer-agency
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/operator       ./cmd/operator
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/patent-extractor ./cmd/patent-extractor
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/market-analysis ./cmd/market-analysis
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/patent-screen ./cmd/patent-screen
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/patent-pipeline ./cmd/patent-pipeline

# ---- Runtime stage ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /out/techtransfer-agency   /usr/local/bin/techtransfer-agency
COPY --from=builder /out/operator       /usr/local/bin/operator
COPY --from=builder /out/patent-extractor /usr/local/bin/patent-extractor
COPY --from=builder /out/market-analysis /usr/local/bin/market-analysis
COPY --from=builder /out/patent-screen /usr/local/bin/patent-screen
COPY --from=builder /out/patent-pipeline /usr/local/bin/patent-pipeline

# Static web assets for the operator UI
COPY web/ /app/web/

WORKDIR /app
