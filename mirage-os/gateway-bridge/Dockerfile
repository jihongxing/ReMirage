FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app

# 先复制 mirage-proto（共享依赖）
COPY mirage-proto/ /mirage-proto/

# 复制 gateway-bridge 源码
COPY mirage-os/gateway-bridge/go.mod mirage-os/gateway-bridge/go.sum ./
RUN sed -i 's|=> ../../mirage-proto|=> /mirage-proto|' go.mod
RUN go mod download
COPY mirage-os/gateway-bridge/ .
RUN CGO_ENABLED=0 go build -o /gateway-bridge ./cmd/bridge/main.go

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /gateway-bridge .
COPY --from=builder /app/configs ./configs
EXPOSE 50051
CMD ["./gateway-bridge"]
