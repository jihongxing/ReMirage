// gen-test-token: 生成本地冒烟测试用的 bootstrap token
package main

import (
	"fmt"
	"os"
	"time"

	"phantom-client/pkg/token"
)

func main() {
	gatewayIP := "127.0.0.1"
	if len(os.Args) > 1 {
		gatewayIP = os.Args[1]
	}

	port := 8443
	if len(os.Args) > 2 {
		fmt.Sscanf(os.Args[2], "%d", &port)
	}
	key := make([]byte, 32) // zero key for testing

	config := &token.BootstrapConfig{
		BootstrapPool: []token.GatewayEndpoint{
			{IP: gatewayIP, Port: port, Region: "test-local"},
		},
		AuthKey:         make([]byte, 32),
		PreSharedKey:    make([]byte, 32),
		CertFingerprint: "test",
		UserID:          "smoke-test",
		ExpiresAt:       time.Now().Add(24 * time.Hour),
	}

	tokenStr, err := token.TokenToBase64(config, key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(tokenStr)
}
