package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// PhantomStatus Phantom 欺骗引擎状态
type PhantomStatus struct {
	Enabled       bool           `json:"enabled"`
	PersonaName   string         `json:"persona_name"`
	PersonaDomain string         `json:"persona_domain"`
	HoneypotCount int            `json:"honeypot_count"`
	TrapCount     int64          `json:"trap_count"`
	LabyrinthHits int64          `json:"labyrinth_hits"`
	ActiveTraps   []TrapInfo     `json:"active_traps,omitempty"`
	Canaries      []CanaryStatus `json:"canaries,omitempty"`
}

// TrapInfo 蜜罐信息
type TrapInfo struct {
	IP        string `json:"ip"`
	RiskLevel int    `json:"risk_level"`
	Hits      int64  `json:"hits"`
	LastHit   string `json:"last_hit"`
}

// CanaryStatus 金丝雀状态
type CanaryStatus struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	Triggers int64  `json:"triggers"`
}

var phantomCmd = &cobra.Command{
	Use:   "phantom",
	Short: "Phantom 欺骗引擎",
	Long:  "查看 Phantom 影子欺骗引擎状态、蜜罐、金丝雀、迷宫统计",
}

var phantomStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "欺骗引擎状态",
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := gatewayGet("/api/phantom/status")
		if err != nil {
			return err
		}

		var status PhantomStatus
		if err := json.Unmarshal(body, &status); err != nil {
			printRawJSON(body)
			return nil
		}

		if outputJSON {
			printJSON(status)
			return nil
		}

		icon := "✅"
		if !status.Enabled {
			icon = "❌"
		}

		fmt.Printf("%s Phantom 欺骗引擎\n", icon)
		fmt.Printf("  伪装身份:    %s (%s)\n", status.PersonaName, status.PersonaDomain)
		fmt.Printf("  蜜罐数量:    %d\n", status.HoneypotCount)
		fmt.Printf("  捕获次数:    %d\n", status.TrapCount)
		fmt.Printf("  迷宫命中:    %d\n", status.LabyrinthHits)

		if len(status.ActiveTraps) > 0 {
			fmt.Println("\n  🍯 活跃蜜罐:")
			for _, t := range status.ActiveTraps {
				fmt.Printf("    %-16s  风险=%d  命中=%d  最后: %s\n",
					t.IP, t.RiskLevel, t.Hits, t.LastHit)
			}
		}

		if len(status.Canaries) > 0 {
			fmt.Println("\n  🐤 金丝雀:")
			for _, c := range status.Canaries {
				cIcon := "🟢"
				if c.Status != "active" {
					cIcon = "🔴"
				}
				fmt.Printf("    %s %-12s  %-10s  触发=%d\n",
					cIcon, c.ID, c.Type, c.Triggers)
			}
		}

		return nil
	},
}

var phantomTrapsCmd = &cobra.Command{
	Use:   "traps",
	Short: "列出蜜罐详情",
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := gatewayGet("/api/phantom/traps")
		if err != nil {
			return err
		}
		printRawJSON(body)
		return nil
	},
}

var phantomPersonaCmd = &cobra.Command{
	Use:   "persona",
	Short: "查看当前伪装身份",
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := gatewayGet("/api/phantom/persona")
		if err != nil {
			return err
		}

		if outputJSON {
			printRawJSON(body)
			return nil
		}

		var persona map[string]interface{}
		if err := json.Unmarshal(body, &persona); err != nil {
			fmt.Println(string(body))
			return nil
		}

		fmt.Println("🎭 伪装身份 (Persona)")
		fmt.Println()
		printRawJSON(body)
		return nil
	},
}

func init() {
	phantomCmd.AddCommand(phantomStatusCmd)
	phantomCmd.AddCommand(phantomTrapsCmd)
	phantomCmd.AddCommand(phantomPersonaCmd)
}
