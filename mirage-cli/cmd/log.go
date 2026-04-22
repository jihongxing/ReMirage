package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// LogEntry 日志条目
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Module    string `json:"module"`
	Message   string `json:"message"`
}

var (
	logLimit  int
	logLevel  string
	logModule string
)

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "日志查看与导出",
	Long:  "查看 Gateway 内存日志（Mirage 不写磁盘日志，仅保留内存环形缓冲区）",
}

var logShowCmd = &cobra.Command{
	Use:   "show",
	Short: "查看最近日志",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := fmt.Sprintf("/api/logs?limit=%d", logLimit)
		if logLevel != "" {
			path += "&level=" + logLevel
		}
		if logModule != "" {
			path += "&module=" + logModule
		}

		body, err := gatewayGet(path)
		if err != nil {
			return err
		}

		if outputJSON {
			printRawJSON(body)
			return nil
		}

		var entries []LogEntry
		if err := json.Unmarshal(body, &entries); err != nil {
			fmt.Println(string(body))
			return nil
		}

		if len(entries) == 0 {
			fmt.Println("暂无日志")
			return nil
		}

		for _, e := range entries {
			icon := logLevelIcon(e.Level)
			fmt.Printf("%s [%s] %-8s %-12s %s\n",
				icon, e.Timestamp, e.Level, e.Module, e.Message)
		}
		fmt.Printf("\n共 %d 条\n", len(entries))
		return nil
	},
}

var logAuditCmd = &cobra.Command{
	Use:   "audit",
	Short: "查看命令审计日志",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := fmt.Sprintf("/api/logs/audit?limit=%d", logLimit)
		body, err := gatewayGet(path)
		if err != nil {
			return err
		}

		if outputJSON {
			printRawJSON(body)
			return nil
		}

		fmt.Println("📋 命令审计日志")
		fmt.Println()
		printRawJSON(body)
		return nil
	},
}

var logStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "日志统计",
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := gatewayGet("/api/logs/stats")
		if err != nil {
			return err
		}

		if outputJSON {
			printRawJSON(body)
			return nil
		}

		fmt.Println("📊 日志统计")
		fmt.Println()
		printRawJSON(body)
		return nil
	},
}

func init() {
	logShowCmd.Flags().IntVarP(&logLimit, "limit", "n", 50, "显示条数")
	logShowCmd.Flags().StringVar(&logLevel, "level", "", "过滤日志级别 (debug/info/warn/error)")
	logShowCmd.Flags().StringVar(&logModule, "module", "", "过滤模块名")
	logAuditCmd.Flags().IntVarP(&logLimit, "limit", "n", 50, "显示条数")

	logCmd.AddCommand(logShowCmd)
	logCmd.AddCommand(logAuditCmd)
	logCmd.AddCommand(logStatsCmd)
}

func logLevelIcon(level string) string {
	switch level {
	case "error":
		return "🔴"
	case "warn":
		return "🟡"
	case "info":
		return "🔵"
	case "debug":
		return "⚪"
	default:
		return "  "
	}
}
