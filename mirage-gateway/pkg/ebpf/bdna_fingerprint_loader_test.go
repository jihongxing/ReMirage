package ebpf

import (
	"path/filepath"
	"runtime"
	"testing"
)

func getConfigPath() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "configs", "bdna", "fingerprints.yaml")
}

func TestFingerprintTemplateCount(t *testing.T) {
	fps, err := LoadFingerprintsFromYAML(getConfigPath())
	if err != nil {
		t.Fatalf("加载指纹配置失败: %v", err)
	}

	if len(fps) < 30 {
		t.Fatalf("指纹模板数量 %d < 30", len(fps))
	}
	t.Logf("✅ 指纹模板数量: %d", len(fps))
}

func TestFingerprintBrowserCoverage(t *testing.T) {
	fps, err := LoadFingerprintsFromYAML(getConfigPath())
	if err != nil {
		t.Fatalf("加载指纹配置失败: %v", err)
	}

	required := map[string]bool{
		"Chrome":  false,
		"Firefox": false,
		"Safari":  false,
		"Edge":    false,
	}

	for _, fp := range fps {
		if _, ok := required[fp.Browser]; ok {
			required[fp.Browser] = true
		}
	}

	for browser, found := range required {
		if !found {
			t.Errorf("缺少浏览器覆盖: %s", browser)
		}
	}
}

func TestFingerprintProfileIDsUnique(t *testing.T) {
	fps, err := LoadFingerprintsFromYAML(getConfigPath())
	if err != nil {
		t.Fatalf("加载指纹配置失败: %v", err)
	}

	seen := make(map[uint32]string)
	for _, fp := range fps {
		if prev, ok := seen[fp.ProfileID]; ok {
			t.Errorf("重复 profile_id=%d: %s 和 %s", fp.ProfileID, prev, fp.ProfileName)
		}
		seen[fp.ProfileID] = fp.ProfileName
	}
}
