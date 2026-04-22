package stego

import (
	"encoding/binary"

	pb "mirage-proto/gen"
)

// StegoDecoder is the steganography decoder for Scheme B.
type StegoDecoder struct {
	sessionKey []byte
}

// NewStegoDecoder creates a new stego decoder.
func NewStegoDecoder(sessionKey []byte) *StegoDecoder {
	return &StegoDecoder{sessionKey: sessionKey}
}

// Decode attempts to extract a ControlCommand from a packet.
// Returns (nil, nil) if HMAC doesn't match (normal dummy packet).
// Returns (nil, err) if HMAC matches but decryption fails.
// Returns (cmd, nil) on success.
func (d *StegoDecoder) Decode(packet []byte) (*pb.ControlCommand, error) {
	return ParseStegoPayload(d.sessionKey, packet)
}

// IsStego performs constant-time HMAC verification to check if packet is stego.
func (d *StegoDecoder) IsStego(packet []byte) bool {
	if len(packet) < MinStegoPayloadOverhead {
		return false
	}
	tag := packet[:HMACTagSize]
	rest := packet[HMACTagSize:]
	if len(rest) < 2 {
		return false
	}
	ctLen := int(binary.BigEndian.Uint16(rest[0:2]))
	innerLen := 2 + ctLen
	if innerLen > len(rest) {
		return false
	}
	inner := rest[:innerLen]
	return HMACVerify(d.sessionKey, inner, tag)
}
