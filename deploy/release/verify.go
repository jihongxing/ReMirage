package release

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
)

// VerifyManifest 验证 manifest 签名和二进制 hash
func VerifyManifest(m *ReleaseManifest, publicKey ed25519.PublicKey, binaryPath string) error {
	// 1. 验证签名
	payload := m.Version + m.BuildTime + m.GitCommit + m.BinarySHA256
	sig, err := hex.DecodeString(m.Signature)
	if err != nil {
		return fmt.Errorf("签名解码失败: %w", err)
	}
	if !ed25519.Verify(publicKey, []byte(payload), sig) {
		return fmt.Errorf("签名验证失败：manifest 可能被篡改")
	}

	// 2. 验证二进制 hash
	actualHash, err := ComputeBinaryHash(binaryPath)
	if err != nil {
		return fmt.Errorf("计算二进制 hash 失败: %w", err)
	}
	if actualHash != m.BinarySHA256 {
		return fmt.Errorf("二进制 hash 不匹配: expected=%s, actual=%s", m.BinarySHA256, actualHash)
	}

	return nil
}
