package cmd

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"net"
	"os"

	"github.com/spf13/cobra"
)

const (
	gatewaySocketPath = "/var/run/mirage-gateway.sock"
	mirageKeyDir      = ".mirage"
	privateKeyFile    = "private.key"
)

var signCmd = &cobra.Command{
	Use:   "sign <challenge>",
	Short: "对挑战码进行硬件签名",
	Long:  "使用本地 Ed25519 私钥对 Mirage-OS 下发的挑战码进行签名",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		challenge := args[0]

		privateKey, err := loadPrivateKey()
		if err != nil {
			return fmt.Errorf("加载私钥失败: %w\n提示: 请先运行 'mirage-cli keygen' 生成密钥对", err)
		}

		signature := ed25519.Sign(privateKey, []byte(challenge))
		signatureHex := hex.EncodeToString(signature)

		fmt.Println("═══════════════════════════════════════════════════════")
		fmt.Println("🔐 Hardware Signature:")
		fmt.Println("═══════════════════════════════════════════════════════")
		fmt.Println(signatureHex)
		fmt.Println("═══════════════════════════════════════════════════════")

		return nil
	},
}

var keygenCmd = &cobra.Command{
	Use:   "keygen",
	Short: "生成 Ed25519 密钥对",
	Long:  "生成新的 Ed25519 密钥对用于影子认证",
	RunE: func(cmd *cobra.Command, args []string) error {
		publicKey, privateKey, err := ed25519.GenerateKey(nil)
		if err != nil {
			return fmt.Errorf("生成密钥对失败: %w", err)
		}

		if err := savePrivateKey(privateKey); err != nil {
			return fmt.Errorf("保存私钥失败: %w", err)
		}

		publicKeyHex := hex.EncodeToString(publicKey)
		fingerprint := hex.EncodeToString(publicKey[:16])

		fmt.Println("═══════════════════════════════════════════════════════")
		fmt.Println("🔑 密钥对已生成")
		fmt.Println("═══════════════════════════════════════════════════════")
		fmt.Println()
		fmt.Println("📋 公钥 (注册时提交):")
		fmt.Println(publicKeyHex)
		fmt.Println()
		fmt.Printf("🔖 指纹: %s\n", fingerprint)
		fmt.Println()
		fmt.Println("⚠️  私钥已保存到: ~/.mirage/private.key")
		fmt.Println("═══════════════════════════════════════════════════════")

		return nil
	},
}

func loadPrivateKey() (ed25519.PrivateKey, error) {
	// 优先从 Gateway Unix Socket 获取
	if _, err := os.Stat(gatewaySocketPath); err == nil {
		return loadFromGateway()
	}
	return loadFromFile()
}

func loadFromGateway() (ed25519.PrivateKey, error) {
	conn, err := net.Dial("unix", gatewaySocketPath)
	if err != nil {
		return nil, fmt.Errorf("连接 Gateway 失败: %w", err)
	}
	defer conn.Close()

	conn.Write([]byte("GET_PRIVATE_KEY\n"))

	buf := make([]byte, 128)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("读取私钥失败: %w", err)
	}

	return hex.DecodeString(string(buf[:n]))
}

func loadFromFile() (ed25519.PrivateKey, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	keyPath := homeDir + "/" + mirageKeyDir + "/" + privateKeyFile
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("读取私钥文件失败: %w", err)
	}

	return hex.DecodeString(string(data))
}

func savePrivateKey(privateKey ed25519.PrivateKey) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	dir := homeDir + "/" + mirageKeyDir
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	keyPath := dir + "/" + privateKeyFile
	return os.WriteFile(keyPath, []byte(hex.EncodeToString(privateKey)), 0600)
}
