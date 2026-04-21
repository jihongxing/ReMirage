package grpc

import (
	"context"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// CNWhitelistInterceptor 返回一个 gRPC 一元拦截器，校验客户端证书 CN 是否在白名单中。
// 无 peer 信息或无客户端证书时返回 Unauthenticated；CN 不在白名单时返回 PermissionDenied。
func CNWhitelistInterceptor(allowedCNs []string) grpc.UnaryServerInterceptor {
	cnSet := make(map[string]bool, len(allowedCNs))
	for _, cn := range allowedCNs {
		cnSet[cn] = true
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler) (interface{}, error) {

		p, ok := peer.FromContext(ctx)
		if !ok {
			log.Printf("[WARN] gRPC CN whitelist: no peer info for %s", info.FullMethod)
			return nil, status.Errorf(codes.Unauthenticated, "no peer info")
		}

		tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
		if !ok || len(tlsInfo.State.PeerCertificates) == 0 {
			log.Printf("[WARN] gRPC CN whitelist: no client cert for %s", info.FullMethod)
			return nil, status.Errorf(codes.Unauthenticated, "no client certificate")
		}

		cn := tlsInfo.State.PeerCertificates[0].Subject.CommonName
		if !cnSet[cn] {
			log.Printf("[WARN] gRPC CN whitelist: CN %q denied for %s", cn, info.FullMethod)
			return nil, status.Errorf(codes.PermissionDenied, "CN not allowed: %s", cn)
		}

		return handler(ctx, req)
	}
}

// CNWhitelistStreamInterceptor 返回一个 gRPC 流式拦截器，校验逻辑同上。
func CNWhitelistStreamInterceptor(allowedCNs []string) grpc.StreamServerInterceptor {
	cnSet := make(map[string]bool, len(allowedCNs))
	for _, cn := range allowedCNs {
		cnSet[cn] = true
	}

	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo,
		handler grpc.StreamHandler) error {

		p, ok := peer.FromContext(ss.Context())
		if !ok {
			return status.Errorf(codes.Unauthenticated, "no peer info")
		}

		tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
		if !ok || len(tlsInfo.State.PeerCertificates) == 0 {
			return status.Errorf(codes.Unauthenticated, "no client certificate")
		}

		cn := tlsInfo.State.PeerCertificates[0].Subject.CommonName
		if !cnSet[cn] {
			log.Printf("[WARN] gRPC CN whitelist stream: CN %q denied for %s", cn, info.FullMethod)
			return status.Errorf(codes.PermissionDenied, "CN not allowed: %s", cn)
		}

		return handler(srv, ss)
	}
}
