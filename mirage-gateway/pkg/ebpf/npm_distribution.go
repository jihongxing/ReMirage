package ebpf

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
)

type NPMDistributionBin struct {
	CumulativeProb uint32
	PktLenLow      uint16
	PktLenHigh     uint16
}

type baselineDistributionFile struct {
	Bins []struct {
		Low            uint16  `json:"low"`
		High           uint16  `json:"high"`
		Count          uint64  `json:"count"`
		Probability    float64 `json:"probability"`
		CumulativeProb float64 `json:"cumulative_prob"`
	} `json:"bins"`
}

func (da *DefenseApplier) LoadTargetDistribution(baselinePath string) error {
	distMap := da.loader.GetMap("npm_target_distribution_map")
	if distMap == nil {
		return fmt.Errorf("npm_target_distribution_map not found")
	}

	bins, err := BuildNPMDistributionBins(baselinePath)
	if err != nil {
		return err
	}

	for i, bin := range bins {
		key := uint32(i)
		if err := distMap.Put(&key, &bin); err != nil {
			return fmt.Errorf("write npm_target_distribution_map[%d]: %w", i, err)
		}
	}

	return nil
}

func BuildNPMDistributionBins(path string) ([]NPMDistributionBin, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read distribution %q: %w", path, err)
	}

	var file baselineDistributionFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse distribution %q: %w", path, err)
	}
	if len(file.Bins) == 0 {
		return nil, fmt.Errorf("distribution %q has no bins", path)
	}
	if len(file.Bins) > 256 {
		return nil, fmt.Errorf("distribution %q has %d bins, max 256", path, len(file.Bins))
	}

	var total uint64
	for _, bin := range file.Bins {
		total += bin.Count
	}

	bins := make([]NPMDistributionBin, 0, len(file.Bins))
	var cumulative uint32
	var cumulativeFloat float64
	for i, bin := range file.Bins {
		if bin.High < bin.Low {
			return nil, fmt.Errorf("bin %d high < low", i)
		}

		if total > 0 {
			cumulativeFloat += float64(bin.Count) / float64(total)
			cumulative = uint32(math.Round(cumulativeFloat * 10000))
		} else if bin.CumulativeProb > 0 {
			cumulative = uint32(math.Round(bin.CumulativeProb * 10000))
		} else {
			cumulativeFloat += bin.Probability
			cumulative = uint32(math.Round(cumulativeFloat * 10000))
		}

		if cumulative > 10000 {
			cumulative = 10000
		}
		bins = append(bins, NPMDistributionBin{
			CumulativeProb: cumulative,
			PktLenLow:      bin.Low,
			PktLenHigh:     bin.High,
		})
	}

	if bins[len(bins)-1].CumulativeProb == 0 {
		return nil, fmt.Errorf("distribution %q cumulative probability is zero", path)
	}
	bins[len(bins)-1].CumulativeProb = 10000

	return bins, nil
}

func ReadMergedIATStats(path string) (mean uint32, sigma uint32, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, fmt.Errorf("open IAT stats %q: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return 0, 0, fmt.Errorf("read IAT stats %q: %w", path, err)
	}
	if len(records) < 2 {
		return 0, 0, fmt.Errorf("IAT stats %q has no data rows", path)
	}

	header := map[string]int{}
	for i, name := range records[0] {
		header[name] = i
	}

	meanIdx, okMean := header["iat_mean_us"]
	stdIdx, okStd := header["iat_std_us"]
	if !okMean || !okStd {
		return 0, 0, fmt.Errorf("IAT stats %q missing iat_mean_us/iat_std_us", path)
	}

	parse := func(idx int) (uint32, error) {
		v, err := strconv.ParseFloat(records[1][idx], 64)
		if err != nil {
			return 0, err
		}
		if v < 0 {
			v = 0
		}
		return uint32(math.Round(v)), nil
	}

	mean, err = parse(meanIdx)
	if err != nil {
		return 0, 0, fmt.Errorf("parse iat_mean_us: %w", err)
	}
	sigma, err = parse(stdIdx)
	if err != nil {
		return 0, 0, fmt.Errorf("parse iat_std_us: %w", err)
	}
	return mean, sigma, nil
}
