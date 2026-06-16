# syntax=docker/dockerfile:1
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/indexer ./cmd/indexer

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/indexer /indexer
USER nonroot:nonroot
ENTRYPOINT ["/indexer"]
