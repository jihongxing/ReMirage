package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func TestCNWhitelistInterceptor_AllowedCN(t *testing.T) {
	interceptor := CNWhitelistInterceptor([]string{"mirage-os", "mirage-bridge"})

	ctx := peerCtxWithCN("mirage-os")
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	}

	resp, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/test"}, handler)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp != "ok" {
		t.Fatalf("expected 'ok', got %v", resp)
	}
}

func TestCNWhitelistInterceptor_DeniedCN(t *testing.T) {
	interceptor := CNWhitelistInterceptor([]string{"mirage-os"})

	ctx := peerCtxWithCN("evil-client")
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	}

	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/test"}, handler)
	if err == nil {
		t.Fatal("expected error for denied CN")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got %v", err)
	}
}

func TestCNWhitelistInterceptor_NoPeer(t *testing.T) {
	interceptor := CNWhitelistInterceptor([]string{"mirage-os"})

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	}

	_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/test"}, handler)
	if err == nil {
		t.Fatal("expected error for no peer")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", err)
	}
}

func TestCNWhitelistInterceptor_NoCert(t *testing.T) {
	interceptor := CNWhitelistInterceptor([]string{"mirage-os"})

	// peer with TLS info but no certs
	p := &peer.Peer{
		AuthInfo: credentials.TLSInfo{
			State: tls.ConnectionState{PeerCertificates: nil},
		},
	}
	ctx := peer.NewContext(context.Background(), p)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	}

	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/test"}, handler)
	if err == nil {
		t.Fatal("expected error for no cert")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", err)
	}
}

// helper: create context with peer TLS info containing a cert with given CN
func peerCtxWithCN(cn string) context.Context {
	cert := &x509.Certificate{
		Subject: pkix.Name{CommonName: cn},
	}
	p := &peer.Peer{
		AuthInfo: credentials.TLSInfo{
			State: tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{cert},
			},
		},
	}
	return peer.NewContext(context.Background(), p)
}
