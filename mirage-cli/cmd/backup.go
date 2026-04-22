package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	backupOutputDir string
	backupEncrypt   bool
	backupGPGRecip  string
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Gateway 状态备份",
	Long:  "备份 Gateway 运行状态快照（配置、证书指纹、eBPF Map 快照、网络状态）",
}

var backupCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建状态备份",
	RunE: func(cmd *cobra.Command, args []string) error {
		if runtime.GOOS != "linux" {
			return fmt.Errorf("状态备份仅支持 Linux")
		}

		scriptPaths := []string{
			"/opt/mirage/scripts/backup-state.sh",
			"/usr/local/bin/backup-state.sh",
			"deploy/scripts/backup-state.sh",
		}

		var scriptPath string
		for _, p := range scriptPaths {
			if _, err := os.Stat(p); err == nil {
				scriptPath = p
				break
			}
		}
		if scriptPath == "" {
			return fmt.Errorf("找不到 backup-state.sh 脚本")
		}

		backupArgs := []string{scriptPath, "--output-dir", backupOutputDir}
		if backupEncrypt {
			backupArgs = append(backupArgs, "--encrypt")
			if backupGPGRecip != "" {
				backupArgs = append(backupArgs, "--gpg-recipient", backupGPGRecip)
			}
		}

		fmt.Printf("创建备份到: %s\n", backupOutputDir)
		execCmd := exec.Command("bash", backupArgs...)
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr
		return execCmd.Run()
	},
}

var backupListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出已有备份",
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := os.ReadDir(backupOutputDir)
		if err != nil {
			return fmt.Errorf("无法读取备份目录 (%s): %w", backupOutputDir, err)
		}

		found := false
		fmt.Printf("备份目录: %s\n\n", backupOutputDir)
		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				continue
			}
			if !e.IsDir() {
				fmt.Printf("  📦 %-45s  %8d bytes  %s\n",
					e.Name(), info.Size(), info.ModTime().Format("2006-01-02 15:04:05"))
				found = true
			}
		}
		if !found {
			fmt.Println("  (无备份文件)")
		}
		return nil
	},
}

func init() {
	backupCmd.PersistentFlags().StringVar(&backupOutputDir, "output-dir", "/var/backups/mirage", "备份输出目录")
	backupCreateCmd.Flags().BoolVar(&backupEncrypt, "encrypt", false, "GPG 加密备份")
	backupCreateCmd.Flags().StringVar(&backupGPGRecip, "gpg-recipient", "", "GPG 接收者")

	backupCmd.AddCommand(backupCreateCmd)
	backupCmd.AddCommand(backupListCmd)
}
