// Package release - 发布产物签名与验证
package release

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
)

// ReleaseManifest 发布清单
type ReleaseManifest struct {
	Version      string `json:"version"`
	BuildTime    string `json:"build_time"`
	GitCommit    string `json:"git_commit"`
	BinarySHA256 string `json:"binary_sha256"`
	Signature    string `json:"signature"` // Ed25519 签名（hex）
}

// LoadManifest 从文件加载 manifest
func LoadManifest(path string) (*ReleaseManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 manifest 失败: %w", err)
	}
	var m ReleaseManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("解析 manifest 失败: %w", err)
	}
	return &m, nil
}

// ComputeBinaryHash 计算二进制文件 SHA-256
func ComputeBinaryHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取二进制文件失败: %w", err)
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// SignManifest 对 manifest 签名
func SignManifest(m *ReleaseManifest, privateKey ed25519.PrivateKey) error {
	payload := m.Version + m.BuildTime + m.GitCommit + m.BinarySHA256
	sig := ed25519.Sign(privateKey, []byte(payload))
	m.Signature = hex.EncodeToString(sig)
	return nil
}

// SaveManifest 保存 manifest 到文件
func SaveManifest(m *ReleaseManifest, path string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 manifest 失败: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}
