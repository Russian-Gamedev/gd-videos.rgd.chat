FROM golang:1.26-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/tg-channel-parser ./cmd/api/main.go

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /app/tg-channel-parser .

EXPOSE 8090

VOLUME ["/app/pb_data"]

ENTRYPOINT ["/app/tg-channel-parser"]
CMD ["serve", "--http=0.0.0.0:8090", "--dir=/app/pb_data"]