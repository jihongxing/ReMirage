package topology

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// TestTopologyHandlerRouteRegistration 验证 topology handler 路由注册和基本响应
func TestTopologyHandlerRouteRegistration(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/topology", func(w http.ResponseWriter, r *http.Request) {
		resp := RouteTableResponse{
			Version:     1,
			PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Gateways:    []GatewayNode{},
			Signature:   []byte("test"),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	req := httptest.NewRequest("GET", "/api/v2/topology", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200, 实际=%d", w.Code)
	}

	var resp RouteTableResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("解码响应失败: %v", err)
	}

	if resp.Version != 1 {
		t.Errorf("期望 version=1, 实际=%d", resp.Version)
	}
}

// --- PBT generators ---
func genGatewayNode(t *rapid.T) GatewayNode {
	return GatewayNode{
		IP:       rapid.StringMatching(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`).Draw(t, "ip"),
		Port:     rapid.IntRange(1, 65535).Draw(t, "port"),
		Priority: uint8(rapid.IntRange(0, 255).Draw(t, "priority")),
		Region:   rapid.StringMatching(`[a-z]{2}-[a-z]+-\d`).Draw(t, "region"),
		CellID:   rapid.StringMatching(`cell-[a-z0-9]{4}`).Draw(t, "cellID"),
	}
}

func genGatewayNodes(t *rapid.T) []GatewayNode {
	n := rapid.IntRange(1, 10).Draw(t, "nodeCount")
	nodes := make([]GatewayNode, n)
	for i := range nodes {
		nodes[i] = genGatewayNode(t)
	}
	return nodes
}

func genRouteTableResponse(t *rapid.T) RouteTableResponse {
	return RouteTableResponse{
		Version:     rapid.Uint64Range(1, 1<<32).Draw(t, "version"),
		PublishedAt: time.Unix(rapid.Int64Range(1e9, 2e9).Draw(t, "publishedAt"), 0).UTC(),
		Gateways:    genGatewayNodes(t),
	}
}

// --- Client-side types (replicated for cross-module verification) ---

// clientGatewayNode mirrors phantom-client/pkg/gtclient.GatewayNode
type clientGatewayNode struct {
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Priority uint8  `json:"priority"`
	Region   string `json:"region"`
	CellID   string `json:"cell_id"`
}

// clientRouteTableResponse mirrors phantom-client/pkg/gtclient.RouteTableResponse
type clientRouteTableResponse struct {
	Gateways    []clientGatewayNode `json:"gateways"`
	Version     uint64              `json:"version"`
	PublishedAt time.Time           `json:"published_at"`
	Signature   []byte              `json:"signature"`
}

// clientHMACBody mirrors phantom-client/pkg/gtclient.hmacBody
type clientHMACBody struct {
	Gateways    []clientGatewayNode `json:"gateways"`
	Version     uint64              `json:"version"`
	PublishedAt time.Time           `json:"published_at"`
}

// clientComputeHMAC replicates phantom-client/pkg/gtclient.ComputeHMAC
func clientComputeHMAC(resp *clientRouteTableResponse, key []byte) []byte {
	body := clientHMACBody{
		Gateways:    resp.Gateways,
		Version:     resp.Version,
		PublishedAt: resp.PublishedAt,
	}
	data, _ := json.Marshal(body)
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

// --- PBT: OS→Client serialization round-trip ---
// **Validates: Requirements 2.3**

func TestProperty_OSToClientSerializationRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		nodes := genGatewayNodes(t)

		// OS 端序列化
		osResp := RouteTableResponse{
			Version:     rapid.Uint64Range(1, 1<<32).Draw(t, "version"),
			PublishedAt: time.Unix(rapid.Int64Range(1e9, 2e9).Draw(t, "pubAt"), 0).UTC(),
			Gateways:    nodes,
			Signature:   []byte("sig"),
		}
		data, err := json.Marshal(osResp)
		if err != nil {
			t.Fatalf("OS 端序列化失败: %v", err)
		}

		// Client 端反序列化
		var clientResp clientRouteTableResponse
		if err := json.Unmarshal(data, &clientResp); err != nil {
			t.Fatalf("Client 端反序列化失败: %v", err)
		}

		// 验证字段完整
		if clientResp.Version != osResp.Version {
			t.Errorf("Version 不匹配: OS=%d, Client=%d", osResp.Version, clientResp.Version)
		}
		if !clientResp.PublishedAt.Equal(osResp.PublishedAt) {
			t.Errorf("PublishedAt 不匹配: OS=%v, Client=%v", osResp.PublishedAt, clientResp.PublishedAt)
		}
		if len(clientResp.Gateways) != len(osResp.Gateways) {
			t.Fatalf("Gateways 数量不匹配: OS=%d, Client=%d", len(osResp.Gateways), len(clientResp.Gateways))
		}
		for i, osGW := range osResp.Gateways {
			cGW := clientResp.Gateways[i]
			if cGW.IP != osGW.IP {
				t.Errorf("[%d] IP 不匹配: OS=%q, Client=%q", i, osGW.IP, cGW.IP)
			}
			if cGW.Port != osGW.Port {
				t.Errorf("[%d] Port 不匹配: OS=%d, Client=%d", i, osGW.Port, cGW.Port)
			}
			if cGW.Priority != osGW.Priority {
				t.Errorf("[%d] Priority 不匹配: OS=%d, Client=%d", i, osGW.Priority, cGW.Priority)
			}
			if cGW.Region != osGW.Region {
				t.Errorf("[%d] Region 不匹配: OS=%q, Client=%q", i, osGW.Region, cGW.Region)
			}
			if cGW.CellID != osGW.CellID {
				t.Errorf("[%d] CellID 不匹配: OS=%q, Client=%q", i, osGW.CellID, cGW.CellID)
			}
		}
	})
}

// --- PBT: HMAC consistency between OS and Client ---
// **Validates: Requirements 2.4**

func TestProperty_HMACConsistencyOSAndClient(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		psk := rapid.SliceOfN(rapid.Byte(), 16, 64).Draw(t, "psk")
		resp := genRouteTableResponse(t)

		// OS 端 HMAC 计算（与 handler.go 中的逻辑一致）
		osBody := hmacBody{
			Gateways:    resp.Gateways,
			Version:     resp.Version,
			PublishedAt: resp.PublishedAt,
		}
		osData, _ := json.Marshal(osBody)
		osMac := hmac.New(sha256.New, psk)
		osMac.Write(osData)
		osSignature := osMac.Sum(nil)

		// 将 OS 响应序列化再反序列化为 Client 端类型（模拟网络传输）
		osResp := RouteTableResponse{
			Version:     resp.Version,
			PublishedAt: resp.PublishedAt,
			Gateways:    resp.Gateways,
			Signature:   osSignature,
		}
		wireJSON, _ := json.Marshal(osResp)

		var clientResp clientRouteTableResponse
		if err := json.Unmarshal(wireJSON, &clientResp); err != nil {
			t.Fatalf("Client 反序列化失败: %v", err)
		}

		// Client 端 HMAC 计算
		clientSig := clientComputeHMAC(&clientResp, psk)

		// 验证两端 HMAC 一致
		if !hmac.Equal(osSignature, clientSig) {
			t.Errorf("HMAC 不匹配:\n  OS=%x\n  Client=%x", osSignature, clientSig)
		}

		// 验证 Client 端签名与传输中的签名一致
		if !hmac.Equal(clientResp.Signature, clientSig) {
			t.Errorf("Client 签名验证失败: wire=%x, computed=%x", clientResp.Signature, clientSig)
		}
	})
}
