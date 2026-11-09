FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY . .
RUN go build -o /routing ./cmd/routing

FROM alpine:latest
WORKDIR /app
COPY --from=builder /routing .
EXPOSE 8081
CMD ["./routing"]
