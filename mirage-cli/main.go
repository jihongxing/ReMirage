// Package main - Mirage CLI 用户端工具
// 用于影子认证的硬件签名生成
package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"runtime"
)

// 版本元数据（编译时注入）
var (
	Version   = "1.0.0"
	BuildTime = "unknown"
	GitCommit = "unknown"
	GoVersion = runtime.Version()
)

const (
	// GatewaySocketPath Gateway Unix Socket 路径
	GatewaySocketPath = "/var/run/mirage-gateway.sock"
	
	// 产品信息
	ProductName   = "Mirage CLI"
	ProductVendor = "Mirage Project"
	ProductURL    = "https://github.com/mirage-project"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]

	switch cmd {
	case "sign":
		if len(os.Args) < 3 {
			fmt.Println("❌ 缺少挑战码参数")
			fmt.Println("用法: mirage-cli sign <challenge>")
			os.Exit(1)
		}
		signChallenge(os.Args[2])

	case "keygen":
		generateKeyPair()

	case "fingerprint":
		if len(os.Args) < 3 {
			fmt.Println("❌ 缺少公钥参数")
			fmt.Println("用法: mirage-cli fingerprint <public_key_hex>")
			os.Exit(1)
		}
		showFingerprint(os.Args[2])

	case "version":
		fmt.Printf("%s v%s\n", ProductName, Version)
		fmt.Printf("  Build Time: %s\n", BuildTime)
		fmt.Printf("  Git Commit: %s\n", GitCommit)
		fmt.Printf("  Go Version: %s\n", GoVersion)
		fmt.Printf("  OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
		fmt.Printf("  Vendor:     %s\n", ProductVendor)

	default:
		printUsage()
		os.Exit(1)
	}
}

// printUsage 打印使用说明
func printUsage() {
	fmt.Println(`
Mirage CLI - 影子认证工具

用法:
  mirage-cli <command> [arguments]

命令:
  sign <challenge>       对挑战码进行硬件签名
  keygen                 生成新的 Ed25519 密钥对
  fingerprint <pubkey>   显示公钥指纹
  version                显示版本信息

示例:
  # 1. 生成密钥对（首次使用）
  mirage-cli keygen

  # 2. 对登录挑战进行签名
  mirage-cli sign "mirage-auth:user123:1699999999:abc123..."

  # 3. 查看公钥指纹
  mirage-cli fingerprint "a1b2c3d4..."`)
}

// signChallenge 对挑战码进行签名
func signChallenge(challenge string) {
	// 1. 尝试从 Gateway 获取私钥
	privateKey, err := loadPrivateKey()
	if err != nil {
		fmt.Printf("❌ 加载私钥失败: %v\n", err)
		fmt.Println("提示: 请先运行 'mirage-cli keygen' 生成密钥对")
		os.Exit(1)
	}

	// 2. 签名
	message := []byte(challenge)
	signature := ed25519.Sign(privateKey, message)

	// 3. 输出 Hex 签名
	signatureHex := hex.EncodeToString(signature)
	
	fmt.Println("═══════════════════════════════════════════════════════")
	fmt.Println("🔐 Hardware Signature (复制到 Web 登录界面):")
	fmt.Println("═══════════════════════════════════════════════════════")
	fmt.Println(signatureHex)
	fmt.Println("═══════════════════════════════════════════════════════")
}

// generateKeyPair 生成新的密钥对
func generateKeyPair() {
	// 1. 生成 Ed25519 密钥对
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		fmt.Printf("❌ 生成密钥对失败: %v\n", err)
		os.Exit(1)
	}

	// 2. 保存私钥
	if err := savePrivateKey(privateKey); err != nil {
		fmt.Printf("❌ 保存私钥失败: %v\n", err)
		os.Exit(1)
	}

	// 3. 输出公钥
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
	fmt.Println("⚠️  请妥善保管，切勿泄露！")
	fmt.Println("═══════════════════════════════════════════════════════")
}

// showFingerprint 显示公钥指纹
func showFingerprint(publicKeyHex string) {
	publicKey, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		fmt.Printf("❌ 公钥格式错误: %v\n", err)
		os.Exit(1)
	}

	if len(publicKey) < 16 {
		fmt.Println("❌ 公钥长度不足")
		os.Exit(1)
	}

	fingerprint := hex.EncodeToString(publicKey[:16])
	fmt.Printf("🔖 指纹: %s\n", fingerprint)
}

// loadPrivateKey 加载私钥
func loadPrivateKey() (ed25519.PrivateKey, error) {
	// 1. 尝试从 Gateway Unix Socket 获取
	if _, err := os.Stat(GatewaySocketPath); err == nil {
		return loadFromGateway()
	}

	// 2. 从本地文件加载
	return loadFromFile()
}

// loadFromGateway 从 Gateway 获取私钥
func loadFromGateway() (ed25519.PrivateKey, error) {
	conn, err := net.Dial("unix", GatewaySocketPath)
	if err != nil {
		return nil, fmt.Errorf("连接 Gateway 失败: %w", err)
	}
	defer conn.Close()

	// 发送获取私钥请求
	conn.Write([]byte("GET_PRIVATE_KEY\n"))

	// 读取响应
	buf := make([]byte, 128)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("读取私钥失败: %w", err)
	}

	privateKeyHex := string(buf[:n])
	return hex.DecodeString(privateKeyHex)
}

// loadFromFile 从本地文件加载私钥
func loadFromFile() (ed25519.PrivateKey, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	keyPath := homeDir + "/.mirage/private.key"
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("读取私钥文件失败: %w", err)
	}

	return hex.DecodeString(string(data))
}

// savePrivateKey 保存私钥
func savePrivateKey(privateKey ed25519.PrivateKey) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	mirageDir := homeDir + "/.mirage"
	if err := os.MkdirAll(mirageDir, 0700); err != nil {
		return err
	}

	keyPath := mirageDir + "/private.key"
	privateKeyHex := hex.EncodeToString(privateKey)

	return os.WriteFile(keyPath, []byte(privateKeyHex), 0600)
}
