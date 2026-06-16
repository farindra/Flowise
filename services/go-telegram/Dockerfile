# ── Stage 1: Build ───────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -trimpath -o go-telegram .

# ── Stage 2: Run ─────────────────────────────────────────────────────────────
FROM alpine:3.19

# ca-certificates diperlukan untuk HTTPS calls ke Telegram & Flowise
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /app/go-telegram .

EXPOSE 8081

CMD ["./go-telegram"]
