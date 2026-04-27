package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// Feature: phase4-northstar-verdict, Property 1: VerifyEvidenceCompleteness correctness
func TestProperty_VerifyEvidenceCompleteness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		numItems := rapid.IntRange(0, 20).Draw(rt, "numItems")
		items := make([]EvidenceItem, numItems)
		tmpDir := t.TempDir()

		// Track which required files exist and which don't
		var expectedMissingRequired []int

		for i := 0; i < numItems; i++ {
			items[i] = EvidenceItem{
				Domain:    rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "domain"),
				Milestone: rapid.SampledFrom([]string{"M1", "M2", "M3", "M4", "M5", "M6", "M7", "M8", "M9", "M10", "M11", "M12"}).Draw(rt, "milestone"),
				Path:      rapid.StringMatching(`[a-z]{1,5}/[a-z]{1,5}\.md`).Draw(rt, "path"),
				Required:  rapid.Bool().Draw(rt, "required"),
			}
			fileExists := rapid.Bool().Draw(rt, "exists")
			if fileExists {
				fullPath := filepath.Join(tmpDir, items[i].Path)
				os.MkdirAll(filepath.Dir(fullPath), 0755)
				os.WriteFile(fullPath, []byte("test"), 0644)
			} else if items[i].Required {
				expectedMissingRequired = append(expectedMissingRequired, i)
			}
		}

		manifest := &EvidenceManifest{Items: items}
		result, err := VerifyEvidenceCompleteness(manifest, tmpDir)

		// Property: error iff at least one required file missing
		if len(expectedMissingRequired) > 0 {
			if err == nil {
				rt.Fatalf("expected error for %d missing required files, got nil", len(expectedMissingRequired))
			}
			// error message must contain all missing required file paths
			for _, idx := range expectedMissingRequired {
				if !strings.Contains(err.Error(), items[idx].Path) {
					rt.Fatalf("error message missing path %q", items[idx].Path)
				}
			}
		} else {
			if err != nil {
				rt.Fatalf("expected nil error (no required missing), got: %v", err)
			}
		}

		// Optional missing never causes error
		if result == nil {
			rt.Fatal("result should never be nil")
		}
	})
}

func TestVerifyEvidenceCompleteness_AllPresent(t *testing.T) {
	tmpDir := t.TempDir()
	manifest := &EvidenceManifest{
		Items: []EvidenceItem{
			{Domain: "d1", Milestone: "M1", Path: "a.md", Required: true},
			{Domain: "d2", Milestone: "M2", Path: "b.md", Required: false},
		},
	}
	for _, item := range manifest.Items {
		p := filepath.Join(tmpDir, item.Path)
		os.WriteFile(p, []byte("ok"), 0644)
	}
	result, err := VerifyEvidenceCompleteness(manifest, tmpDir)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if len(result.MissingRequired) != 0 || len(result.MissingOptional) != 0 {
		t.Fatal("expected no missing files")
	}
}

func TestVerifyEvidenceCompleteness_RequiredMissing(t *testing.T) {
	tmpDir := t.TempDir()
	manifest := &EvidenceManifest{
		Items: []EvidenceItem{
			{Domain: "test-domain", Milestone: "M1", Path: "missing.md", Required: true},
		},
	}
	_, err := VerifyEvidenceCompleteness(manifest, tmpDir)
	if err == nil {
		t.Fatal("expected error for missing required file")
	}
	if !strings.Contains(err.Error(), "missing.md") {
		t.Fatalf("error should contain path, got: %v", err)
	}
	if !strings.Contains(err.Error(), "test-domain") {
		t.Fatalf("error should contain domain, got: %v", err)
	}
}

func TestVerifyEvidenceCompleteness_OptionalMissing(t *testing.T) {
	tmpDir := t.TempDir()
	manifest := &EvidenceManifest{
		Items: []EvidenceItem{
			{Domain: "d1", Milestone: "M1", Path: "optional.md", Required: false},
		},
	}
	result, err := VerifyEvidenceCompleteness(manifest, tmpDir)
	if err != nil {
		t.Fatalf("optional missing should not cause error, got: %v", err)
	}
	if len(result.MissingOptional) != 1 {
		t.Fatalf("expected 1 missing optional, got %d", len(result.MissingOptional))
	}
}

func TestVerifyEvidenceCompleteness_EmptyManifest(t *testing.T) {
	result, err := VerifyEvidenceCompleteness(&EvidenceManifest{}, t.TempDir())
	if err != nil {
		t.Fatalf("empty manifest should return nil error, got: %v", err)
	}
	if len(result.MissingRequired) != 0 {
		t.Fatal("empty manifest should have no missing")
	}
}

func TestVerifyEvidenceCompleteness_NilManifest(t *testing.T) {
	result, err := VerifyEvidenceCompleteness(nil, t.TempDir())
	if err != nil {
		t.Fatalf("nil manifest should return nil error, got: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestDefaultEvidenceManifest_CoversM1ToM12(t *testing.T) {
	m := DefaultEvidenceManifest()
	milestones := make(map[string]bool)
	for _, item := range m.Items {
		milestones[item.Milestone] = true
	}
	expected := []string{"M1", "M2", "M3", "M4", "M5", "M6", "M7", "M8", "M9", "M10", "M11", "M12"}
	for _, ms := range expected {
		if !milestones[ms] {
			t.Errorf("default manifest missing milestone %s", ms)
		}
	}
}
