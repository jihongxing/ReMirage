// control_command.pb.go — 手写 Protobuf 兼容实现
// 对应 control_command.proto，在 protoc 可用时应重新生成
// 使用 protowire 实现二进制编解码，兼容标准 Protobuf 线格式

package gen

import (
	"errors"
	"math"

	"google.golang.org/protobuf/encoding/protowire"
)

// ControlCommandType 控制指令类型枚举
type ControlCommandType int32

const (
	ControlCommandType_CONTROL_COMMAND_UNKNOWN ControlCommandType = 0
	ControlCommandType_PERSONA_FLIP            ControlCommandType = 1
	ControlCommandType_BUDGET_SYNC             ControlCommandType = 2
	ControlCommandType_SURVIVAL_MODE_CHANGE    ControlCommandType = 3
	ControlCommandType_ROLLBACK                ControlCommandType = 4
	ControlCommandType_SESSION_MIGRATE         ControlCommandType = 5
)

var ControlCommandType_name = map[int32]string{
	0: "CONTROL_COMMAND_UNKNOWN",
	1: "PERSONA_FLIP",
	2: "BUDGET_SYNC",
	3: "SURVIVAL_MODE_CHANGE",
	4: "ROLLBACK",
	5: "SESSION_MIGRATE",
}

var ControlCommandType_value = map[string]int32{
	"CONTROL_COMMAND_UNKNOWN": 0,
	"PERSONA_FLIP":            1,
	"BUDGET_SYNC":             2,
	"SURVIVAL_MODE_CHANGE":    3,
	"ROLLBACK":                4,
	"SESSION_MIGRATE":         5,
}

func (x ControlCommandType) String() string {
	if name, ok := ControlCommandType_name[int32(x)]; ok {
		return name
	}
	return "CONTROL_COMMAND_UNKNOWN"
}

// ControlCommand Protobuf 控制指令消息
type ControlCommand struct {
	CommandId   string             `json:"command_id,omitempty"`
	CommandType ControlCommandType `json:"command_type,omitempty"`
	Epoch       uint64             `json:"epoch,omitempty"`
	Timestamp   int64              `json:"timestamp,omitempty"`
	Payload     []byte             `json:"payload,omitempty"`
}

func (x *ControlCommand) GetCommandId() string {
	if x != nil {
		return x.CommandId
	}
	return ""
}

func (x *ControlCommand) GetCommandType() ControlCommandType {
	if x != nil {
		return x.CommandType
	}
	return ControlCommandType_CONTROL_COMMAND_UNKNOWN
}

func (x *ControlCommand) GetEpoch() uint64 {
	if x != nil {
		return x.Epoch
	}
	return 0
}

func (x *ControlCommand) GetTimestamp() int64 {
	if x != nil {
		return x.Timestamp
	}
	return 0
}

func (x *ControlCommand) GetPayload() []byte {
	if x != nil {
		return x.Payload
	}
	return nil
}

// MarshalControlCommand 序列化为 Protobuf 二进制线格式
func MarshalControlCommand(cmd *ControlCommand) ([]byte, error) {
	if cmd == nil {
		return nil, errors.New("nil ControlCommand")
	}
	var buf []byte
	// field 1: command_id (string, tag=1, wire type 2)
	if cmd.CommandId != "" {
		buf = protowire.AppendTag(buf, 1, protowire.BytesType)
		buf = protowire.AppendString(buf, cmd.CommandId)
	}
	// field 2: command_type (enum/varint, tag=2, wire type 0)
	if cmd.CommandType != 0 {
		buf = protowire.AppendTag(buf, 2, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(cmd.CommandType))
	}
	// field 3: epoch (uint64, tag=3, wire type 0)
	if cmd.Epoch != 0 {
		buf = protowire.AppendTag(buf, 3, protowire.VarintType)
		buf = protowire.AppendVarint(buf, cmd.Epoch)
	}
	// field 4: timestamp (int64, tag=4, wire type 0)
	if cmd.Timestamp != 0 {
		buf = protowire.AppendTag(buf, 4, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(cmd.Timestamp))
	}
	// field 5: payload (bytes, tag=5, wire type 2)
	if len(cmd.Payload) > 0 {
		buf = protowire.AppendTag(buf, 5, protowire.BytesType)
		buf = protowire.AppendBytes(buf, cmd.Payload)
	}
	return buf, nil
}

// UnmarshalControlCommand 从 Protobuf 二进制线格式反序列化
func UnmarshalControlCommand(data []byte) (*ControlCommand, error) {
	cmd := &ControlCommand{}
	for len(data) > 0 {
		num, wtype, n := protowire.ConsumeTag(data)
		if n < 0 {
			return nil, errors.New("invalid protobuf tag")
		}
		data = data[n:]
		switch num {
		case 1: // command_id
			if wtype != protowire.BytesType {
				return nil, errors.New("invalid wire type for command_id")
			}
			v, n := protowire.ConsumeString(data)
			if n < 0 {
				return nil, errors.New("invalid command_id")
			}
			cmd.CommandId = v
			data = data[n:]
		case 2: // command_type
			if wtype != protowire.VarintType {
				return nil, errors.New("invalid wire type for command_type")
			}
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return nil, errors.New("invalid command_type")
			}
			cmd.CommandType = ControlCommandType(v)
			data = data[n:]
		case 3: // epoch
			if wtype != protowire.VarintType {
				return nil, errors.New("invalid wire type for epoch")
			}
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return nil, errors.New("invalid epoch")
			}
			cmd.Epoch = v
			data = data[n:]
		case 4: // timestamp
			if wtype != protowire.VarintType {
				return nil, errors.New("invalid wire type for timestamp")
			}
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return nil, errors.New("invalid timestamp")
			}
			cmd.Timestamp = int64(v)
			data = data[n:]
		case 5: // payload
			if wtype != protowire.BytesType {
				return nil, errors.New("invalid wire type for payload")
			}
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return nil, errors.New("invalid payload")
			}
			cmd.Payload = append([]byte(nil), v...)
			data = data[n:]
		default:
			// skip unknown fields
			n := protowire.ConsumeFieldValue(num, wtype, data)
			if n < 0 {
				return nil, errors.New("invalid unknown field")
			}
			data = data[n:]
		}
	}
	return cmd, nil
}

// 确保 math 包被使用
var _ = math.MaxFloat64

// ControlCommandAck Protobuf 控制指令确认消息
type ControlCommandAck struct {
	CommandId    string `json:"command_id,omitempty"`
	Success      bool   `json:"success,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

func (x *ControlCommandAck) GetCommandId() string {
	if x != nil {
		return x.CommandId
	}
	return ""
}

func (x *ControlCommandAck) GetSuccess() bool {
	if x != nil {
		return x.Success
	}
	return false
}

func (x *ControlCommandAck) GetErrorMessage() string {
	if x != nil {
		return x.ErrorMessage
	}
	return ""
}

// MarshalControlCommandAck 序列化 ControlCommandAck
func MarshalControlCommandAck(ack *ControlCommandAck) ([]byte, error) {
	if ack == nil {
		return nil, errors.New("nil ControlCommandAck")
	}
	var buf []byte
	if ack.CommandId != "" {
		buf = protowire.AppendTag(buf, 1, protowire.BytesType)
		buf = protowire.AppendString(buf, ack.CommandId)
	}
	if ack.Success {
		buf = protowire.AppendTag(buf, 2, protowire.VarintType)
		buf = protowire.AppendVarint(buf, 1)
	}
	if ack.ErrorMessage != "" {
		buf = protowire.AppendTag(buf, 3, protowire.BytesType)
		buf = protowire.AppendString(buf, ack.ErrorMessage)
	}
	return buf, nil
}

// UnmarshalControlCommandAck 反序列化 ControlCommandAck
func UnmarshalControlCommandAck(data []byte) (*ControlCommandAck, error) {
	ack := &ControlCommandAck{}
	for len(data) > 0 {
		num, wtype, n := protowire.ConsumeTag(data)
		if n < 0 {
			return nil, errors.New("invalid protobuf tag")
		}
		data = data[n:]
		switch num {
		case 1:
			if wtype != protowire.BytesType {
				return nil, errors.New("invalid wire type for command_id")
			}
			v, n := protowire.ConsumeString(data)
			if n < 0 {
				return nil, errors.New("invalid command_id")
			}
			ack.CommandId = v
			data = data[n:]
		case 2:
			if wtype != protowire.VarintType {
				return nil, errors.New("invalid wire type for success")
			}
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return nil, errors.New("invalid success")
			}
			ack.Success = v != 0
			data = data[n:]
		case 3:
			if wtype != protowire.BytesType {
				return nil, errors.New("invalid wire type for error_message")
			}
			v, n := protowire.ConsumeString(data)
			if n < 0 {
				return nil, errors.New("invalid error_message")
			}
			ack.ErrorMessage = v
			data = data[n:]
		default:
			n := protowire.ConsumeFieldValue(num, wtype, data)
			if n < 0 {
				return nil, errors.New("invalid unknown field")
			}
			data = data[n:]
		}
	}
	return ack, nil
}
