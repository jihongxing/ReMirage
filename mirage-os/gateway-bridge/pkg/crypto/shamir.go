package crypto

import (
	"crypto/rand"
	"fmt"
)

// GF(256) 预计算表
var gfExp [512]byte
var gfLog [256]byte

func init() {
	// AES 不可约多项式 x^8 + x^4 + x^3 + x + 1 = 0x11B
	// 使用生成元 3（primitive root）
	x := 1
	for i := 0; i < 255; i++ {
		gfExp[i] = byte(x)
		gfLog[x] = byte(i)
		x = (x << 1) ^ x // x * 3 = x * 2 + x
		if x >= 256 {
			x ^= 0x11B
		}
	}
	for i := 255; i < 512; i++ {
		gfExp[i] = gfExp[i-255]
	}
}

func gfAdd(a, b byte) byte { return a ^ b }

func gfMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return gfExp[int(gfLog[a])+int(gfLog[b])]
}

func gfInv(a byte) byte {
	if a == 0 {
		return 0
	}
	return gfExp[255-int(gfLog[a])]
}

func gfDiv(a, b byte) byte {
	if b == 0 {
		return 0
	}
	if a == 0 {
		return 0
	}
	return gfMul(a, gfInv(b))
}

// Share Shamir 份额
type Share struct {
	X byte   // x 坐标（1-255）
	Y []byte // y 值数组（与密钥等长）
}

// ShamirEngine Shamir 秘密分享引擎
type ShamirEngine struct {
	threshold int
	total     int
}

// NewShamirEngine 创建引擎
func NewShamirEngine(threshold, total int) (*ShamirEngine, error) {
	if threshold < 2 {
		return nil, fmt.Errorf("threshold must be >= 2, got %d", threshold)
	}
	if total > 255 {
		return nil, fmt.Errorf("total must be <= 255, got %d", total)
	}
	if threshold > total {
		return nil, fmt.Errorf("threshold (%d) must be <= total (%d)", threshold, total)
	}
	return &ShamirEngine{threshold: threshold, total: total}, nil
}

// Split 将密钥拆分为 N 个份额
func (se *ShamirEngine) Split(secret []byte) ([]Share, error) {
	if len(secret) == 0 {
		return nil, fmt.Errorf("secret must not be empty")
	}

	shares := make([]Share, se.total)
	for i := range shares {
		shares[i] = Share{X: byte(i + 1), Y: make([]byte, len(secret))}
	}

	// 对每个字节生成随机多项式
	coeffs := make([]byte, se.threshold-1)
	for byteIdx, secretByte := range secret {
		// 生成 threshold-1 个随机系数
		if _, err := rand.Read(coeffs); err != nil {
			return nil, fmt.Errorf("generate random coefficients: %w", err)
		}

		// 计算 f(x) = secret + a1*x + a2*x^2 + ...
		for i := 0; i < se.total; i++ {
			x := byte(i + 1)
			y := secretByte
			xPow := x
			for _, coeff := range coeffs {
				y = gfAdd(y, gfMul(coeff, xPow))
				xPow = gfMul(xPow, x)
			}
			shares[i].Y[byteIdx] = y
		}
	}

	return shares, nil
}

// Combine 从份额恢复密钥
func (se *ShamirEngine) Combine(shares []Share) ([]byte, error) {
	if len(shares) < se.threshold {
		return nil, fmt.Errorf("need at least %d shares, got %d", se.threshold, len(shares))
	}

	// 检查重复 x 坐标
	seen := make(map[byte]bool)
	for _, s := range shares {
		if seen[s.X] {
			return nil, fmt.Errorf("duplicate x coordinate: %d", s.X)
		}
		seen[s.X] = true
	}

	// 取前 threshold 个份额
	shares = shares[:se.threshold]
	secretLen := len(shares[0].Y)
	secret := make([]byte, secretLen)

	// 拉格朗日插值 GF(256)
	for byteIdx := 0; byteIdx < secretLen; byteIdx++ {
		var value byte
		for i := 0; i < se.threshold; i++ {
			xi := shares[i].X
			yi := shares[i].Y[byteIdx]

			// 计算拉格朗日基多项式 l_i(0)
			num := byte(1)
			den := byte(1)
			for j := 0; j < se.threshold; j++ {
				if i == j {
					continue
				}
				xj := shares[j].X
				num = gfMul(num, xj)            // 0 - xj = xj (GF(256))
				den = gfMul(den, gfAdd(xi, xj)) // xi - xj
			}
			value = gfAdd(value, gfMul(yi, gfDiv(num, den)))
		}
		secret[byteIdx] = value
	}

	return secret, nil
}
