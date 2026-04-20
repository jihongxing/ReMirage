package cellular

import (
	"testing"

	pb "mirage-os/api/proto"
	"mirage-os/pkg/models"

	"pgregory.net/rapid"
)

// Feature: mirage-os-completion, Property 1: RegisterCell 输入验证完整性
// **Validates: Requirements 2.1**
func TestProperty_RegisterCellInputValidation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cellID := rapid.StringMatching(`[a-z0-9]{0,10}`).Draw(t, "cell_id")
		levelInt := rapid.IntRange(0, 4).Draw(t, "level")
		hasLocation := rapid.Bool().Draw(t, "has_location")
		country := rapid.StringMatching(`[A-Z]{0,3}`).Draw(t, "country")

		var loc *pb.GeoLocation
		if hasLocation {
			loc = &pb.GeoLocation{Country: country}
		}

		req := &pb.RegisterCellRequest{
			CellId:   cellID,
			Level:    pb.CellLevel(levelInt),
			Location: loc,
		}

		err := ValidateRegisterCellRequest(req)

		isInvalid := cellID == "" ||
			pb.CellLevel(levelInt) == pb.CellLevel_LEVEL_UNKNOWN ||
			loc == nil || country == ""

		if isInvalid {
			if err == nil {
				t.Fatalf("expected error for invalid input: cellID=%q level=%d loc=%v country=%q",
					cellID, levelInt, hasLocation, country)
			}
		} else {
			if err != nil {
				t.Fatalf("unexpected error for valid input: %v", err)
			}
		}
	})
}

// Feature: mirage-os-completion, Property 2: ListCells 筛选条件正确性
// **Validates: Requirements 2.2**
func TestProperty_ListCellsFilterCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		numCells := rapid.IntRange(0, 20).Draw(t, "num_cells")
		cells := make([]CellWithLoad, numCells)
		for i := range cells {
			cells[i] = CellWithLoad{
				Cell: models.Cell{
					CellID:    rapid.StringMatching(`cell-[a-z]{3}`).Draw(t, "cell_id"),
					CellLevel: rapid.IntRange(1, 3).Draw(t, "cell_level"),
					Country:   rapid.SampledFrom([]string{"US", "CH", "SG", "IS", "PA"}).Draw(t, "country"),
					Status:    rapid.SampledFrom([]string{"active", "maintenance", "offline"}).Draw(t, "status"),
				},
				GatewayCount: rapid.IntRange(0, 100).Draw(t, "gw_count"),
				MaxGateways:  100,
			}
			cells[i].LoadPercent = float32(cells[i].GatewayCount) / float32(cells[i].MaxGateways) * 100
		}

		filterLevel := pb.CellLevel(rapid.IntRange(0, 3).Draw(t, "filter_level"))
		filterCountry := rapid.SampledFrom([]string{"", "US", "CH", "SG"}).Draw(t, "filter_country")
		onlineOnly := rapid.Bool().Draw(t, "online_only")

		result := FilterCells(cells, filterLevel, filterCountry, onlineOnly)

		for _, c := range result {
			if filterLevel != pb.CellLevel_LEVEL_UNKNOWN && c.Cell.CellLevel != int(filterLevel) {
				t.Fatalf("cell level %d does not match filter %d", c.Cell.CellLevel, filterLevel)
			}
			if filterCountry != "" && c.Cell.Country != filterCountry {
				t.Fatalf("cell country %s does not match filter %s", c.Cell.Country, filterCountry)
			}
			if onlineOnly && c.Cell.Status != "active" {
				t.Fatalf("cell status %s is not active with online_only=true", c.Cell.Status)
			}
		}
	})
}

// Feature: mirage-os-completion, Property 3: AllocateGateway 最优选择
// **Validates: Requirements 2.3**
func TestProperty_AllocateGatewayOptimalSelection(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		numCells := rapid.IntRange(1, 15).Draw(t, "num_cells")
		cells := make([]CellWithLoad, numCells)
		for i := range cells {
			cells[i] = CellWithLoad{
				Cell: models.Cell{
					CellID:    rapid.StringMatching(`cell-[a-z]{3}[0-9]`).Draw(t, "cell_id"),
					CellLevel: rapid.IntRange(1, 3).Draw(t, "cell_level"),
					Country:   rapid.SampledFrom([]string{"US", "CH", "SG", "IS"}).Draw(t, "country"),
					Status:    rapid.SampledFrom([]string{"active", "offline"}).Draw(t, "status"),
				},
				LoadPercent: float32(rapid.IntRange(0, 100).Draw(t, "load")),
			}
		}

		prefLevel := pb.CellLevel(rapid.IntRange(0, 3).Draw(t, "pref_level"))
		prefCountry := rapid.SampledFrom([]string{"", "US", "CH"}).Draw(t, "pref_country")

		result, err := SelectBestCell(cells, prefLevel, prefCountry)

		// 手动计算期望结果
		filtered := FilterCells(cells, prefLevel, prefCountry, true)

		if len(filtered) == 0 {
			if err == nil {
				t.Fatal("expected error when no cells match")
			}
			return
		}

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// 验证结果是负载最低的
		for _, c := range filtered {
			if c.LoadPercent < result.LoadPercent {
				t.Fatalf("found cell with lower load %.2f < %.2f", c.LoadPercent, result.LoadPercent)
			}
		}
	})
}

// Feature: mirage-os-completion, Property 4: SwitchCell 目标蜂窝约束
// **Validates: Requirements 2.5**
func TestProperty_SwitchCellTargetConstraints(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		numCells := rapid.IntRange(1, 15).Draw(t, "num_cells")
		jurisdictions := []string{"IS", "CH", "SG", "PA", "SC"}
		cells := make([]CellWithLoad, numCells)
		for i := range cells {
			cells[i] = CellWithLoad{
				Cell: models.Cell{
					CellID:       rapid.StringMatching(`cell-[a-z]{4}`).Draw(t, "cell_id"),
					CellLevel:    rapid.IntRange(1, 3).Draw(t, "cell_level"),
					Jurisdiction: rapid.SampledFrom(jurisdictions).Draw(t, "jurisdiction"),
					Status:       rapid.SampledFrom([]string{"active", "offline"}).Draw(t, "status"),
				},
				LoadPercent: float32(rapid.IntRange(0, 100).Draw(t, "load")),
			}
		}

		currentIdx := rapid.IntRange(0, numCells-1).Draw(t, "current_idx")
		current := cells[currentIdx]

		result, err := SelectSwitchTarget(cells, current.Cell.CellID, current.Cell.CellLevel, current.Cell.Jurisdiction)

		// 手动计算候选
		var candidates []CellWithLoad
		for _, c := range cells {
			if c.Cell.CellID != current.Cell.CellID &&
				c.Cell.CellLevel == current.Cell.CellLevel &&
				c.Cell.Jurisdiction != current.Cell.Jurisdiction &&
				c.Cell.Status == "active" {
				candidates = append(candidates, c)
			}
		}

		if len(candidates) == 0 {
			if err == nil {
				t.Fatal("expected error when no candidates")
			}
			return
		}

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// 验证约束
		if result.Cell.CellID == current.Cell.CellID {
			t.Fatal("target is same as current")
		}
		if result.Cell.CellLevel != current.Cell.CellLevel {
			t.Fatalf("level mismatch: %d != %d", result.Cell.CellLevel, current.Cell.CellLevel)
		}
		if result.Cell.Jurisdiction == current.Cell.Jurisdiction {
			t.Fatal("same jurisdiction")
		}
		if result.Cell.Status != "active" {
			t.Fatal("target not active")
		}
		for _, c := range candidates {
			if c.LoadPercent < result.LoadPercent {
				t.Fatalf("found candidate with lower load %.2f < %.2f", c.LoadPercent, result.LoadPercent)
			}
		}
	})
}
