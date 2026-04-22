package threat

import (
	"testing"
)

// TestGetASNBlockEntries 验证 ThreatIntelProvider 能正确导出 ASN 条目
func TestGetASNBlockEntries(t *testing.T) {
	provider, err := NewThreatIntelProvider(
		"../../configs/asn_database.json",
		"../../configs/cloud_ranges.json",
	)
	if err != nil {
		t.Fatalf("加载威胁情报库失败: %v", err)
	}

	entries := provider.GetASNBlockEntries()
	if len(entries) == 0 {
		t.Fatal("ASN 条目为空，期望非空")
	}

	// 验证条目格式
	for _, e := range entries {
		if e.CIDR == "" {
			t.Error("CIDR 不应为空")
		}
		if e.ASN == 0 {
			t.Errorf("ASN 不应为 0: CIDR=%s", e.CIDR)
		}
	}

	t.Logf("成功导出 %d 条 ASN 条目", len(entries))
}

// TestLookupASNHit 验证已知云厂商 IP 能命中 ASN 查询
func TestLookupASNHit(t *testing.T) {
	provider, err := NewThreatIntelProvider(
		"../../configs/asn_database.json",
		"../../configs/cloud_ranges.json",
	)
	if err != nil {
		t.Fatalf("加载威胁情报库失败: %v", err)
	}

	// 3.0.0.1 属于 AWS (ASN 16509)
	info := provider.LookupASN("3.0.0.1")
	if info == nil {
		t.Fatal("期望命中 ASN 查询，实际未命中")
	}
	if info.ASN != 16509 {
		t.Errorf("期望 ASN=16509, 实际=%d", info.ASN)
	}
}
