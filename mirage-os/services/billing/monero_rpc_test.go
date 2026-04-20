package billing

import (
	"encoding/json"
	"testing"

	"pgregory.net/rapid"
)

// Feature: mirage-os-completion, Property 15: Monero RPC 响应解析
// **Validates: Requirements 12.2**
//
// For any valid JSON-RPC 2.0 response with confirmations field,
// parsed value matches the original confirmations value.
func TestParseGetTransferResponseProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		confirmations := rapid.IntRange(0, 1_000_000).Draw(t, "confirmations")
		amount := rapid.Uint64().Draw(t, "amount")
		txid := rapid.StringMatching(`[0-9a-f]{64}`).Draw(t, "txid")
		address := rapid.StringMatching(`[0-9a-zA-Z]{95}`).Draw(t, "address")

		// 构造有效的 JSON-RPC 2.0 响应
		rpcResp := map[string]any{
			"jsonrpc": "2.0",
			"id":      "0",
			"result": map[string]any{
				"transfer": map[string]any{
					"confirmations": confirmations,
					"amount":        amount,
					"txid":          txid,
					"address":       address,
				},
			},
		}

		data, err := json.Marshal(rpcResp)
		if err != nil {
			t.Fatalf("failed to marshal test response: %v", err)
		}

		result, err := ParseGetTransferResponse(data)
		if err != nil {
			t.Fatalf("ParseGetTransferResponse failed: %v", err)
		}

		if result.Confirmations != confirmations {
			t.Fatalf("confirmations mismatch: expected %d, got %d", confirmations, result.Confirmations)
		}
		if result.TxHash != txid {
			t.Fatalf("txid mismatch: expected %s, got %s", txid, result.TxHash)
		}
	})
}
