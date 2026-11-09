FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY . .
RUN go build -o /search ./cmd/search

FROM alpine:latest
WORKDIR /app
COPY --from=builder /search .
EXPOSE 8082
CMD ["./search"]
