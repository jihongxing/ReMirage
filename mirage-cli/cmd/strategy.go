package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// StrategyStatus 策略状态
type StrategyStatus struct {
	DefenseLevel   int     `json:"defense_level"`
	SecurityState  string  `json:"security_state"`
	JitterMeanUs   uint32  `json:"jitter_mean_us"`
	JitterStddevUs uint32  `json:"jitter_stddev_us"`
	NoiseIntensity uint32  `json:"noise_intensity"`
	PaddingRate    uint32  `json:"padding_rate"`
	TemplateID     uint32  `json:"template_id"`
	AutoAdjust     bool    `json:"auto_adjust"`
	CostPerGB      float64 `json:"cost_per_gb,omitempty"`
}

var strategyCmd = &cobra.Command{
	Use:   "strategy",
	Short: "防御策略管理",
	Long:  "查看和调整防御策略参数（防御等级、Jitter、噪声、Padding 等）",
}

var strategyShowCmd = &cobra.Command{
	Use:   "show",
	Short: "显示当前防御策略",
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := gatewayGet("/api/strategy")
		if err != nil {
			return err
		}

		var status StrategyStatus
		if err := json.Unmarshal(body, &status); err != nil {
			printRawJSON(body)
			return nil
		}

		if outputJSON {
			printJSON(status)
			return nil
		}

		levelIcon := "🟢"
		if status.DefenseLevel >= 4 {
			levelIcon = "🔴"
		} else if status.DefenseLevel >= 3 {
			levelIcon = "🟠"
		} else if status.DefenseLevel >= 2 {
			levelIcon = "🟡"
		}

		fmt.Printf("%s 防御策略\n", levelIcon)
		fmt.Printf("  防御等级:     %d (10=经济, 20=平衡, 30=极限)\n", status.DefenseLevel)
		fmt.Printf("  安全状态:     %s\n", status.SecurityState)
		fmt.Printf("  自动调节:     %v\n", status.AutoAdjust)
		fmt.Println()
		fmt.Println("  协议参数:")
		fmt.Printf("    Jitter 均值:   %d μs\n", status.JitterMeanUs)
		fmt.Printf("    Jitter 标准差: %d μs\n", status.JitterStddevUs)
		fmt.Printf("    噪声强度:      %d\n", status.NoiseIntensity)
		fmt.Printf("    Padding 率:    %d%%\n", status.PaddingRate)
		fmt.Printf("    B-DNA 模板:    %d\n", status.TemplateID)
		if status.CostPerGB > 0 {
			fmt.Printf("    流量成本:      %.2f/GB\n", status.CostPerGB)
		}

		return nil
	},
}

var (
	strategyLevel int
)

var strategySetCmd = &cobra.Command{
	Use:   "set",
	Short: "调整防御等级",
	Long:  "设置防御等级 (10=经济, 20=平衡, 30=极限)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if strategyLevel < 0 || strategyLevel > 30 {
			return fmt.Errorf("防御等级范围: 0-30")
		}

		payload := map[string]interface{}{
			"defense_level": strategyLevel,
		}
		data, _ := json.Marshal(payload)

		body, err := gatewayPost("/api/strategy", "application/json", bytes.NewReader(data))
		if err != nil {
			return err
		}

		fmt.Printf("✅ 防御等级已设置为 %d\n", strategyLevel)
		if len(body) > 2 { // 非空 JSON
			printRawJSON(body)
		}
		return nil
	},
}

var strategyCostCmd = &cobra.Command{
	Use:   "cost",
	Short: "查看防御成本分析",
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := gatewayGet("/api/strategy/cost")
		if err != nil {
			return err
		}

		if outputJSON {
			printRawJSON(body)
			return nil
		}

		var cost map[string]interface{}
		if err := json.Unmarshal(body, &cost); err != nil {
			fmt.Println(string(body))
			return nil
		}

		fmt.Println("💰 防御成本分析")
		fmt.Println()
		printRawJSON(body)
		return nil
	},
}

func init() {
	strategySetCmd.Flags().IntVar(&strategyLevel, "level", 20, "防御等级 (0-30)")

	strategyCmd.AddCommand(strategyShowCmd)
	strategyCmd.AddCommand(strategySetCmd)
	strategyCmd.AddCommand(strategyCostCmd)
}
