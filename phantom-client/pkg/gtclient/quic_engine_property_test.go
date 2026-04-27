package gtclient

import (
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// Feature: zero-signature-elimination, Property 5: QUIC 配置不含自定义协议标识
// **Validates: Requirements 2.1, 2.2, 2.3**

// forbiddenIdentifiers lists custom protocol identifiers that must not appear
// in any QUIC/TLS configuration field.
var forbiddenIdentifiers = []string{
	"mirage",
	"gtunnel",
	"mirage-gtunnel",
	"phantom",
	"gswitch",
}

func TestProperty_QUICConfigNoCustomProtocolIdentifiers(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random but valid QUICEngineConfig inputs
		cfg := &QUICEngineConfig{
			GatewayAddr:    rapid.StringMatching(`^[a-z0-9]+\.[a-z]{2,4}:[0-9]{1,5}$`).Draw(t, "addr"),
			KeepAlive:      time.Duration(rapid.IntRange(0, 60).Draw(t, "keepAlive")) * time.Second,
			RecvBufferSize: rapid.IntRange(0, 8192).Draw(t, "bufSize"),
		}

		// Optionally generate a 32-byte pinned cert hash
		usePinnedCert := rapid.Bool().Draw(t, "usePinnedCert")
		if usePinnedCert {
			cfg.PinnedCertHash = rapid.SliceOfN(rapid.Byte(), 32, 32).Draw(t, "pinnedCertHash")
		}

		// Optionally generate a PSK
		pskLen := rapid.IntRange(0, 64).Draw(t, "pskLen")
		if pskLen > 0 {
			cfg.PSK = rapid.SliceOfN(rapid.Byte(), pskLen, pskLen).Draw(t, "psk")
		}

		engine := NewQUICEngine(cfg)

		// Verify: NextProtos must be exactly ["h3"]
		if len(engine.tlsConf.NextProtos) != 1 {
			t.Fatalf("expected exactly 1 NextProto, got %d: %v", len(engine.tlsConf.NextProtos), engine.tlsConf.NextProtos)
		}
		if engine.tlsConf.NextProtos[0] != "h3" {
			t.Fatalf("expected NextProto 'h3', got %q", engine.tlsConf.NextProtos[0])
		}

		// Verify: no forbidden identifiers in NextProtos
		for _, proto := range engine.tlsConf.NextProtos {
			lowerProto := strings.ToLower(proto)
			for _, forbidden := range forbiddenIdentifiers {
				if strings.Contains(lowerProto, forbidden) {
					t.Fatalf("NextProto %q contains forbidden identifier %q", proto, forbidden)
				}
			}
		}

		// Verify: no forbidden identifiers in ServerName
		if engine.tlsConf.ServerName != "" {
			lowerSN := strings.ToLower(engine.tlsConf.ServerName)
			for _, forbidden := range forbiddenIdentifiers {
				if strings.Contains(lowerSN, forbidden) {
					t.Fatalf("ServerName %q contains forbidden identifier %q", engine.tlsConf.ServerName, forbidden)
				}
			}
		}

		// Verify: gateway address field doesn't contain forbidden identifiers
		lowerAddr := strings.ToLower(engine.addr)
		for _, forbidden := range forbiddenIdentifiers {
			if strings.Contains(lowerAddr, forbidden) {
				t.Fatalf("addr %q contains forbidden identifier %q", engine.addr, forbidden)
			}
		}
	})
}
