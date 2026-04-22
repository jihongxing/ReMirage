//go:build !linux

package dataplane

import "fmt"

// TUNInjector 非 Linux 平台 stub。
type TUNInjector struct{}

// NewTUNInjector 在非 Linux 平台返回错误。
func NewTUNInjector(_ TUNConfig) (*TUNInjector, error) {
	return nil, fmt.Errorf("dataplane: TUN injection requires Linux (current platform unsupported)")
}

// InjectIPPacket stub。
func (t *TUNInjector) InjectIPPacket(_ []byte) error {
	return fmt.Errorf("dataplane: TUN not available on this platform")
}

// Close stub。
func (t *TUNInjector) Close() error { return nil }
