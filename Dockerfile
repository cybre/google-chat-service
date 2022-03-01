FROM golang:1.17.6-alpine AS builder
WORKDIR /app
COPY ./ ./
RUN go mod download
RUN go build ./cmd/producer
RUN go build ./cmd/consumer

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/producer ./
COPY --from=builder /app/consumer ./