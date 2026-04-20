package gtunnel

import (
	"testing"

	"pgregory.net/rapid"
)

// Feature: multi-path-adaptive-transport, Property 1: Shard 序列化往返一致性
func TestProperty_ShardRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 生成合法的 Shard
		index := rapid.IntRange(0, 11).Draw(t, "index") // 0-11 (8 data + 4 parity)
		dataLen := rapid.IntRange(1, ShardSize).Draw(t, "dataLen")
		data := make([]byte, dataLen)
		for i := range data {
			data[i] = byte(rapid.IntRange(0, 255).Draw(t, "byte"))
		}
		isParity := rapid.Bool().Draw(t, "isParity")
		packetID := rapid.Uint64().Draw(t, "packetID")
		epoch := rapid.Uint32().Draw(t, "epoch")

		shard := &Shard{
			Index:    index,
			Data:     data,
			IsParity: isParity,
		}

		// 序列化
		serialized := SerializeShardWithEpoch(shard, packetID, epoch)

		// 反序列化
		restored, restoredPacketID, restoredEpoch, err := DeserializeShardWithEpoch(serialized)
		if err != nil {
			t.Fatalf("反序列化失败: %v", err)
		}

		// 验证往返一致性
		if restored.Index != shard.Index {
			t.Fatalf("Index 不一致: got %d, want %d", restored.Index, shard.Index)
		}
		if restored.IsParity != shard.IsParity {
			t.Fatalf("IsParity 不一致: got %v, want %v", restored.IsParity, shard.IsParity)
		}
		if len(restored.Data) != len(shard.Data) {
			t.Fatalf("Data 长度不一致: got %d, want %d", len(restored.Data), len(shard.Data))
		}
		for i := range shard.Data {
			if restored.Data[i] != shard.Data[i] {
				t.Fatalf("Data[%d] 不一致: got 0x%02x, want 0x%02x", i, restored.Data[i], shard.Data[i])
			}
		}
		if restoredPacketID != packetID {
			t.Fatalf("PacketID 不一致: got %d, want %d", restoredPacketID, packetID)
		}
		if restoredEpoch != epoch {
			t.Fatalf("Epoch 不一致: got %d, want %d", restoredEpoch, epoch)
		}
	})
}

// 验证 DeserializeShard 对过短数据返回错误
func TestDeserializeShard_TooShort(t *testing.T) {
	shortData := []byte{0x01, 0x02, 0x03}
	_, _, err := DeserializeShard(shortData)
	if err == nil {
		t.Fatal("应该返回错误")
	}
}
