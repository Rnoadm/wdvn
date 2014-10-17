// Code generated by protoc-gen-go.
// source: res.proto
// DO NOT EDIT!

/*
Package res is a generated protocol buffer package.

It is generated from these files:
	res.proto

It has these top-level messages:
	Packet
*/
package res

import proto "code.google.com/p/goprotobuf/proto"
import math "math"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = math.Inf

type Type int32

const (
	Type_Ping      Type = 0
	Type_SelectMan Type = 1
	Type_MoveMan   Type = 2
	Type_Input     Type = 3
	Type_FullState Type = 4
)

var Type_name = map[int32]string{
	0: "Ping",
	1: "SelectMan",
	2: "MoveMan",
	3: "Input",
	4: "FullState",
}
var Type_value = map[string]int32{
	"Ping":      0,
	"SelectMan": 1,
	"MoveMan":   2,
	"Input":     3,
	"FullState": 4,
}

func (x Type) Enum() *Type {
	p := new(Type)
	*p = x
	return p
}
func (x Type) String() string {
	return proto.EnumName(Type_name, int32(x))
}
func (x *Type) UnmarshalJSON(data []byte) error {
	value, err := proto.UnmarshalJSONEnum(Type_value, data, "Type")
	if err != nil {
		return err
	}
	*x = Type(value)
	return nil
}

type Man int32

const (
	Man_Whip    Man = 0
	Man_Density Man = 1
	Man_Vacuum  Man = 2
	Man_Normal  Man = 3
	Man_count   Man = 4
)

var Man_name = map[int32]string{
	0: "Whip",
	1: "Density",
	2: "Vacuum",
	3: "Normal",
	4: "count",
}
var Man_value = map[string]int32{
	"Whip":    0,
	"Density": 1,
	"Vacuum":  2,
	"Normal":  3,
	"count":   4,
}

func (x Man) Enum() *Man {
	p := new(Man)
	*p = x
	return p
}
func (x Man) String() string {
	return proto.EnumName(Man_name, int32(x))
}
func (x *Man) UnmarshalJSON(data []byte) error {
	value, err := proto.UnmarshalJSONEnum(Man_value, data, "Man")
	if err != nil {
		return err
	}
	*x = Man(value)
	return nil
}

type Button int32

const (
	Button_released Button = 0
	Button_pressed  Button = 1
)

var Button_name = map[int32]string{
	0: "released",
	1: "pressed",
}
var Button_value = map[string]int32{
	"released": 0,
	"pressed":  1,
}

func (x Button) Enum() *Button {
	p := new(Button)
	*p = x
	return p
}
func (x Button) String() string {
	return proto.EnumName(Button_name, int32(x))
}
func (x *Button) UnmarshalJSON(data []byte) error {
	value, err := proto.UnmarshalJSONEnum(Button_value, data, "Button")
	if err != nil {
		return err
	}
	*x = Button(value)
	return nil
}

type Packet struct {
	Type             *Type   `protobuf:"varint,1,req,name=type,enum=Type" json:"type,omitempty"`
	Man              *Man    `protobuf:"varint,2,opt,name=man,enum=Man" json:"man,omitempty"`
	X                *int64  `protobuf:"zigzag64,3,opt,name=x" json:"x,omitempty"`
	Y                *int64  `protobuf:"zigzag64,4,opt,name=y" json:"y,omitempty"`
	Data             []byte  `protobuf:"bytes,5,opt,name=data" json:"data,omitempty"`
	Mouse1           *Button `protobuf:"varint,16,opt,name=mouse1,enum=Button" json:"mouse1,omitempty"`
	Mouse2           *Button `protobuf:"varint,17,opt,name=mouse2,enum=Button" json:"mouse2,omitempty"`
	KeyUp            *Button `protobuf:"varint,18,opt,name=key_up,enum=Button" json:"key_up,omitempty"`
	KeyDown          *Button `protobuf:"varint,19,opt,name=key_down,enum=Button" json:"key_down,omitempty"`
	KeyLeft          *Button `protobuf:"varint,20,opt,name=key_left,enum=Button" json:"key_left,omitempty"`
	KeyRight         *Button `protobuf:"varint,21,opt,name=key_right,enum=Button" json:"key_right,omitempty"`
	XXX_unrecognized []byte  `json:"-"`
}

func (m *Packet) Reset()         { *m = Packet{} }
func (m *Packet) String() string { return proto.CompactTextString(m) }
func (*Packet) ProtoMessage()    {}

func (m *Packet) GetType() Type {
	if m != nil && m.Type != nil {
		return *m.Type
	}
	return Type_Ping
}

func (m *Packet) GetMan() Man {
	if m != nil && m.Man != nil {
		return *m.Man
	}
	return Man_Whip
}

func (m *Packet) GetX() int64 {
	if m != nil && m.X != nil {
		return *m.X
	}
	return 0
}

func (m *Packet) GetY() int64 {
	if m != nil && m.Y != nil {
		return *m.Y
	}
	return 0
}

func (m *Packet) GetData() []byte {
	if m != nil {
		return m.Data
	}
	return nil
}

func (m *Packet) GetMouse1() Button {
	if m != nil && m.Mouse1 != nil {
		return *m.Mouse1
	}
	return Button_released
}

func (m *Packet) GetMouse2() Button {
	if m != nil && m.Mouse2 != nil {
		return *m.Mouse2
	}
	return Button_released
}

func (m *Packet) GetKeyUp() Button {
	if m != nil && m.KeyUp != nil {
		return *m.KeyUp
	}
	return Button_released
}

func (m *Packet) GetKeyDown() Button {
	if m != nil && m.KeyDown != nil {
		return *m.KeyDown
	}
	return Button_released
}

func (m *Packet) GetKeyLeft() Button {
	if m != nil && m.KeyLeft != nil {
		return *m.KeyLeft
	}
	return Button_released
}

func (m *Packet) GetKeyRight() Button {
	if m != nil && m.KeyRight != nil {
		return *m.KeyRight
	}
	return Button_released
}

func init() {
	proto.RegisterEnum("Type", Type_name, Type_value)
	proto.RegisterEnum("Man", Man_name, Man_value)
	proto.RegisterEnum("Button", Button_name, Button_value)
}
