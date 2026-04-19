//go:build !linux

package crypto

func mlock(_ []byte) error   { return nil }
func munlock(_ []byte) error { return nil }
