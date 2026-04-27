package release

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EvidenceItem 单条证据文件
type EvidenceItem struct {
	Domain    string `json:"domain"`
	Milestone string `json:"milestone"`
	Path      string `json:"path"`
	Required  bool   `json:"required"`
}

// EvidenceManifest 证据清单
type EvidenceManifest struct {
	Items []EvidenceItem `json:"items"`
}

// EvidenceResult 验证结果
type EvidenceResult struct {
	MissingRequired []EvidenceItem
	MissingOptional []EvidenceItem
}

// VerifyEvidenceCompleteness 校验证据完整性
func VerifyEvidenceCompleteness(manifest *EvidenceManifest, rootDir string) (*EvidenceResult, error) {
	result := &EvidenceResult{}
	if manifest == nil || len(manifest.Items) == 0 {
		return result, nil
	}
	for _, item := range manifest.Items {
		fullPath := filepath.Join(rootDir, item.Path)
		_, err := os.Stat(fullPath)
		if err != nil {
			if item.Required {
				result.MissingRequired = append(result.MissingRequired, item)
			} else {
				result.MissingOptional = append(result.MissingOptional, item)
			}
		}
	}
	if len(result.MissingRequired) > 0 {
		var parts []string
		for _, m := range result.MissingRequired {
			parts = append(parts, fmt.Sprintf("[%s] %s", m.Domain, m.Path))
		}
		return result, fmt.Errorf("missing required evidence files: %s", strings.Join(parts, "; "))
	}
	return result, nil
}

// DefaultEvidenceManifest 返回覆盖 M1-M12 全部里程碑的默认清单
func DefaultEvidenceManifest() *EvidenceManifest {
	return &EvidenceManifest{
		Items: []EvidenceItem{
			{Domain: "多承载编排与降级", Milestone: "M1", Path: "docs/governance/carrier-matrix.md", Required: true},
			{Domain: "多承载编排与降级", Milestone: "M2", Path: "deploy/evidence/m2-degradation-drill.log", Required: true},
			{Domain: "节点恢复与共振发现", Milestone: "M3", Path: "deploy/evidence/m3-node-death-drill.log", Required: true},
			{Domain: "会话连续性与链路漂移", Milestone: "M4", Path: "deploy/evidence/m4-continuity-drill.log", Required: true},
			{Domain: "会话连续性与链路漂移", Milestone: "M4", Path: "deploy/evidence/m4-continuity-report.md", Required: true},
			{Domain: "流量整形与特征隐匿", Milestone: "M5", Path: "docs/reports/stealth-experiment-plan.md", Required: true},
			{Domain: "流量整形与特征隐匿", Milestone: "M6", Path: "docs/reports/stealth-experiment-results.md", Required: true},
			{Domain: "流量整形与特征隐匿", Milestone: "M6", Path: "docs/reports/stealth-claims-boundary.md", Required: true},
			{Domain: "eBPF深度参与", Milestone: "M7", Path: "docs/reports/ebpf-coverage-map.md", Required: true},
			{Domain: "反取证与最小运行痕迹", Milestone: "M8", Path: "docs/reports/deployment-tiers.md", Required: true},
			{Domain: "反取证与最小运行痕迹", Milestone: "M8", Path: "docs/reports/deployment-baseline-checklist.md", Required: true},
			{Domain: "准入控制与防滥用", Milestone: "M9", Path: "docs/reports/access-control-joint-drill.md", Required: true},
			{Domain: "全域", Milestone: "M10", Path: "docs/reports/phase4-evidence-audit.md", Required: true},
			{Domain: "全域", Milestone: "M11", Path: "docs/reports/cross-document-consistency.md", Required: true},
			{Domain: "全域", Milestone: "M12", Path: "deploy/release/evidence.go", Required: true},
			{Domain: "全域", Milestone: "M12", Path: "deploy/release/evidence_test.go", Required: true},
			// Optional drill scripts
			{Domain: "多承载编排与降级", Milestone: "M2", Path: "deploy/scripts/drill-m2-degradation.sh", Required: false},
			{Domain: "节点恢复与共振发现", Milestone: "M3", Path: "deploy/scripts/drill-m3-node-death.sh", Required: false},
			{Domain: "会话连续性与链路漂移", Milestone: "M4", Path: "deploy/scripts/drill-m4-continuity.sh", Required: false},
			{Domain: "流量整形与特征隐匿", Milestone: "M6", Path: "deploy/scripts/drill-m6-experiment.sh", Required: false},
			{Domain: "eBPF深度参与", Milestone: "M7", Path: "deploy/scripts/drill-m7-ebpf-coverage.sh", Required: false},
			{Domain: "反取证与最小运行痕迹", Milestone: "M8", Path: "deploy/scripts/drill-m8-baseline.sh", Required: false},
			{Domain: "准入控制与防滥用", Milestone: "M9", Path: "deploy/scripts/drill-m9-joint-drill.sh", Required: false},
		},
	}
}
