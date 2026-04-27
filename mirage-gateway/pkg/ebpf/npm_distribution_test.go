package ebpf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildNPMDistributionBins(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "baseline-distribution-merged.json")
	data := `{"bins":[
{"low":0,"high":99,"count":1},
{"low":100,"high":199,"count":3}
]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write distribution: %v", err)
	}

	bins, err := BuildNPMDistributionBins(path)
	if err != nil {
		t.Fatalf("BuildNPMDistributionBins() error = %v", err)
	}
	if len(bins) != 2 {
		t.Fatalf("len(bins) = %d, want 2", len(bins))
	}
	if bins[0].CumulativeProb != 2500 {
		t.Fatalf("bins[0].CumulativeProb = %d, want 2500", bins[0].CumulativeProb)
	}
	if bins[1].CumulativeProb != 10000 {
		t.Fatalf("bins[1].CumulativeProb = %d, want 10000", bins[1].CumulativeProb)
	}
}

func TestReadMergedIATStats(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "baseline-stats-merged.csv")
	data := "profile_family,iat_mean_us,iat_std_us\nmerged,1234.4,55.6\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write stats: %v", err)
	}

	mean, sigma, err := ReadMergedIATStats(path)
	if err != nil {
		t.Fatalf("ReadMergedIATStats() error = %v", err)
	}
	if mean != 1234 || sigma != 56 {
		t.Fatalf("mean/sigma = %d/%d, want 1234/56", mean, sigma)
	}
}
