FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY . .
RUN go build -o /realtime ./cmd/realtime

FROM alpine:latest
WORKDIR /app
COPY --from=builder /realtime .
EXPOSE 8083
CMD ["./realtime"]
