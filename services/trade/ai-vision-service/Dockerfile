# syntax=docker/dockerfile:1
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/server /server
USER nonroot:nonroot
ENTRYPOINT ["/server"]
