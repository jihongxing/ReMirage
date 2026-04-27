package stego

import (
	"encoding/binary"
	"errors"
	"fmt"

	pb "mirage-proto/gen"

	"google.golang.org/protobuf/proto"
)

// MinStegoPayloadOverhead is HMAC_Tag(32) + ctLen(2) + nonce(12) + auth_tag(16) = 62 bytes minimum overhead.
const MinStegoPayloadOverhead = HMACTagSize + 2 + NonceSize + AuthTagSize

// BuildStegoPayload constructs a stego payload:
// [HMAC_Tag(32) | ctLen(2, big-endian) | Ciphertext | RandomPadding]
// Total length is strictly equal to targetLen.
func BuildStegoPayload(key []byte, cmd *pb.ControlCommand, targetLen int) ([]byte, error) {
	if cmd == nil {
		return nil, errors.New("nil ControlCommand")
	}

	serialized, err := proto.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	ciphertext, err := Encrypt(key, serialized)
	if err != nil {
		return nil, fmt.Errorf("encrypt: %w", err)
	}

	ctLen := len(ciphertext)
	needed := HMACTagSize + 2 + ctLen
	if targetLen < needed {
		return nil, fmt.Errorf("targetLen %d < minimum %d", targetLen, needed)
	}

	// Build inner: [ctLen(2) | ciphertext]
	inner := make([]byte, 2+ctLen)
	binary.BigEndian.PutUint16(inner[0:2], uint16(ctLen))
	copy(inner[2:], ciphertext)

	// HMAC over inner
	tag := HMACTag(key, inner)

	payload := make([]byte, targetLen)
	copy(payload[0:HMACTagSize], tag)
	copy(payload[HMACTagSize:], inner)

	// Random padding
	paddingLen := targetLen - needed
	if paddingLen > 0 {
		padding, err := RandomPadding(paddingLen)
		if err != nil {
			return nil, fmt.Errorf("padding: %w", err)
		}
		copy(payload[needed:], padding)
	}

	return payload, nil
}

// ParseStegoPayload parses a stego payload and extracts the ControlCommand.
// Returns (nil, nil) if HMAC verification fails (normal dummy packet).
// Returns (nil, err) if HMAC matches but decryption fails.
// Returns (cmd, nil) on success.
func ParseStegoPayload(key []byte, payload []byte) (*pb.ControlCommand, error) {
	if len(payload) < MinStegoPayloadOverhead {
		return nil, nil
	}

	tag := payload[:HMACTagSize]
	rest := payload[HMACTagSize:]

	if len(rest) < 2 {
		return nil, nil
	}

	ctLen := int(binary.BigEndian.Uint16(rest[0:2]))
	innerLen := 2 + ctLen
	if innerLen > len(rest) {
		return nil, nil
	}

	inner := rest[:innerLen]
	if !HMACVerify(key, inner, tag) {
		return nil, nil
	}

	ciphertext := inner[2:]
	plaintext, err := Decrypt(key, ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt failed: %w", err)
	}

	cmd := &pb.ControlCommand{}
	err = proto.Unmarshal(plaintext, cmd)
	if err != nil {
		return nil, fmt.Errorf("unmarshal failed: %w", err)
	}

	return cmd, nil
}
