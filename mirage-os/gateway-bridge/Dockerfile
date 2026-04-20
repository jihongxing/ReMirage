FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /gateway-bridge ./cmd/bridge/main.go

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /gateway-bridge .
COPY --from=builder /app/proto ./proto
COPY --from=builder /app/configs ./configs
EXPOSE 50051
CMD ["./gateway-bridge"]
