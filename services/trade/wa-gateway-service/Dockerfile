# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server

# Pure-Go sqlite driver (modernc.org/sqlite) -> CGO_ENABLED=0, jadi bisa pakai
# distroless static. Volume /data dipakai untuk sqlite store (whatsmeow + cart).
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/server /server
USER nonroot:nonroot
ENTRYPOINT ["/server"]
