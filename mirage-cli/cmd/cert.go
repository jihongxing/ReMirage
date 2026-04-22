package cmd

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/spf13/cobra"
)

var (
	certDir       string
	certDays      int
	certNoRestart bool
)

var certCmd = &cobra.Command{
	Use:   "cert",
	Short: "证书管理",
	Long:  "mTLS 证书检查、轮换、详情查看",
}

var certCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "检查证书有效期",
	RunE: func(cmd *cobra.Command, args []string) error {
		certs := []struct {
			name string
			path string
		}{
			{"Gateway 证书", certDir + "/gateway.crt"},
			{"CA 证书", certDir + "/ca.crt"},
		}

		hasExpiring := false
		for _, c := range certs {
			remaining, err := checkCertExpiry(c.path)
			if err != nil {
				fmt.Printf("  ❌ %s: %v\n", c.name, err)
				continue
			}
			days := int(remaining.Hours() / 24)
			icon := "🟢"
			if days <= 0 {
				icon = "🔴"
				hasExpiring = true
			} else if days <= certDays {
				icon = "🟡"
				hasExpiring = true
			}
			fmt.Printf("  %s %s: %d 天后过期\n", icon, c.name, days)
		}

		if hasExpiring {
			fmt.Println("\n⚠️  存在即将过期或已过期的证书，建议执行 mirage-cli cert rotate")
		} else {
			fmt.Println("\n✅ 所有证书有效期充足")
		}
		return nil
	},
}

var certRotateCmd = &cobra.Command{
	Use:   "rotate",
	Short: "轮换 Gateway 证书",
	Long:  "调用 cert-rotate.sh 执行证书轮换（需要 root 权限）",
	RunE: func(cmd *cobra.Command, args []string) error {
		if runtime.GOOS != "linux" {
			return fmt.Errorf("证书轮换仅支持 Linux")
		}

		rotateArgs := []string{"--cert-dir", certDir, "--days-before", fmt.Sprintf("%d", certDays)}
		if certNoRestart {
			rotateArgs = append(rotateArgs, "--no-restart")
		}

		// 尝试多个路径查找脚本
		scriptPaths := []string{
			"/opt/mirage/scripts/cert-rotate.sh",
			"/usr/local/bin/cert-rotate.sh",
			"deploy/scripts/cert-rotate.sh",
		}

		var scriptPath string
		for _, p := range scriptPaths {
			if _, err := os.Stat(p); err == nil {
				scriptPath = p
				break
			}
		}
		if scriptPath == "" {
			return fmt.Errorf("找不到 cert-rotate.sh 脚本")
		}

		fmt.Printf("执行证书轮换: %s\n", scriptPath)
		execCmd := exec.Command("bash", append([]string{scriptPath}, rotateArgs...)...)
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr
		return execCmd.Run()
	},
}

var certInspectCmd = &cobra.Command{
	Use:   "inspect [cert-file]",
	Short: "查看证书详情",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		certPath := certDir + "/gateway.crt"
		if len(args) > 0 {
			certPath = args[0]
		}

		data, err := os.ReadFile(certPath)
		if err != nil {
			return fmt.Errorf("读取证书失败: %w", err)
		}

		block, _ := pem.Decode(data)
		if block == nil {
			return fmt.Errorf("无法解析 PEM 格式")
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return fmt.Errorf("解析证书失败: %w", err)
		}

		remaining := time.Until(cert.NotAfter)
		days := int(remaining.Hours() / 24)

		fmt.Printf("📜 证书详情: %s\n", certPath)
		fmt.Printf("  Subject:     %s\n", cert.Subject.CommonName)
		fmt.Printf("  Issuer:      %s\n", cert.Issuer.CommonName)
		fmt.Printf("  Serial:      %s\n", cert.SerialNumber.Text(16))
		fmt.Printf("  Not Before:  %s\n", cert.NotBefore.Format(time.RFC3339))
		fmt.Printf("  Not After:   %s\n", cert.NotAfter.Format(time.RFC3339))
		fmt.Printf("  剩余天数:    %d 天\n", days)
		fmt.Printf("  算法:        %s\n", cert.SignatureAlgorithm)

		if len(cert.DNSNames) > 0 {
			fmt.Printf("  DNS SANs:    %v\n", cert.DNSNames)
		}
		if len(cert.IPAddresses) > 0 {
			fmt.Printf("  IP SANs:     %v\n", cert.IPAddresses)
		}

		return nil
	},
}

func init() {
	certCmd.PersistentFlags().StringVar(&certDir, "cert-dir", "/var/mirage/certs", "证书目录")
	certCheckCmd.Flags().IntVar(&certDays, "days", 30, "预警天数")
	certRotateCmd.Flags().IntVar(&certDays, "days-before", 30, "提前轮换天数")
	certRotateCmd.Flags().BoolVar(&certNoRestart, "no-restart", false, "轮换后不重启 Gateway")

	certCmd.AddCommand(certCheckCmd)
	certCmd.AddCommand(certRotateCmd)
	certCmd.AddCommand(certInspectCmd)
}

func checkCertExpiry(path string) (time.Duration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("文件不存在: %s", path)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return 0, fmt.Errorf("无法解析 PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return 0, err
	}
	return time.Until(cert.NotAfter), nil
}
