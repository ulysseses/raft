// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: raft.proto

package pb

import (
	context "context"
	fmt "fmt"
	_ "github.com/gogo/protobuf/gogoproto"
	proto "github.com/gogo/protobuf/proto"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	io "io"
	math "math"
	math_bits "math/bits"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion3 // please upgrade the proto package

type MessageType int32

const (
	MsgUnknown MessageType = 0
	// MsgApp Fields
	// * index:    previous entry index
	// * logTerm:  previous entry term
	// * commit:   leader's commit
	// * tid:      read request context if ConsistencyStrict,
	//             lease start if ConsistencyLease,
	//             otherwise 0
	// * proxy:    read request context, otherwise 0 if none
	// * entries:  entries to append, ignored if there is read request context
	MsgApp MessageType = 1
	// MsgAppResp Fields
	// * index:    index of the largest match index,
	// * tid:      read request context if ConsistencyStrict,
	//             lease start if ConsistencyLease,
	//             otherwise 0
	// * proxy:    read request context if ConsistencyStrict, otherwise 0
	// * success:  whether or not the append request was successful
	MsgAppResp MessageType = 2
	// MsgRead Fields
	// * tid: read request context
	MsgRead MessageType = 3
	// MsgReadResp Fields
	// * tid:   read request context
	// * index: read index
	MsgReadResp MessageType = 4
	// MsgProp Fields
	// * tid:      context associated with a proposal request
	// * entries:  contains only 1 entry, of which only the data field is used
	//             to contain the proposed data
	MsgProp MessageType = 5
	// MsgPropResp Fields
	// * tid:      context associated with a proposal request
	// * index:    index of the successfully proposed entry, 0 if unsuccessful
	// * logTerm:  term of the successfully proposed entry, 0 if unsuccessful
	MsgPropResp MessageType = 6
	// MsgVote Fields
	// * index:   index of the candidate's last entry
	// * logTerm: term of the candidate's last entry
	MsgVote MessageType = 7
	// MsgVoteResp Fields (none)
	MsgVoteResp MessageType = 8
)

var MessageType_name = map[int32]string{
	0: "MsgUnknown",
	1: "MsgApp",
	2: "MsgAppResp",
	3: "MsgRead",
	4: "MsgReadResp",
	5: "MsgProp",
	6: "MsgPropResp",
	7: "MsgVote",
	8: "MsgVoteResp",
}

var MessageType_value = map[string]int32{
	"MsgUnknown":  0,
	"MsgApp":      1,
	"MsgAppResp":  2,
	"MsgRead":     3,
	"MsgReadResp": 4,
	"MsgProp":     5,
	"MsgPropResp": 6,
	"MsgVote":     7,
	"MsgVoteResp": 8,
}

func (x MessageType) Enum() *MessageType {
	p := new(MessageType)
	*p = x
	return p
}

func (x MessageType) String() string {
	return proto.EnumName(MessageType_name, int32(x))
}

func (x *MessageType) UnmarshalJSON(data []byte) error {
	value, err := proto.UnmarshalJSONEnum(MessageType_value, data, "MessageType")
	if err != nil {
		return err
	}
	*x = MessageType(value)
	return nil
}

func (MessageType) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_b042552c306ae59b, []int{0}
}

type Message struct {
	Term    uint64      `protobuf:"varint,1,opt,name=term" json:"term"`
	From    uint64      `protobuf:"varint,2,opt,name=from" json:"from"`
	To      uint64      `protobuf:"varint,3,opt,name=to" json:"to"`
	Type    MessageType `protobuf:"varint,4,opt,name=type,enum=pb.MessageType" json:"type"`
	Index   uint64      `protobuf:"varint,5,opt,name=index" json:"index"`
	LogTerm uint64      `protobuf:"varint,6,opt,name=logTerm" json:"logTerm"`
	Commit  uint64      `protobuf:"varint,7,opt,name=commit" json:"commit"`
	Tid     int64       `protobuf:"varint,8,opt,name=tid" json:"tid"`
	Proxy   uint64      `protobuf:"varint,9,opt,name=proxy" json:"proxy"`
	Entries []Entry     `protobuf:"bytes,10,rep,name=entries" json:"entries"`
	Success bool        `protobuf:"varint,11,opt,name=success" json:"success"`
}

func (m *Message) Reset()         { *m = Message{} }
func (m *Message) String() string { return proto.CompactTextString(m) }
func (*Message) ProtoMessage()    {}
func (*Message) Descriptor() ([]byte, []int) {
	return fileDescriptor_b042552c306ae59b, []int{0}
}
func (m *Message) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *Message) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_Message.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *Message) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Message.Merge(m, src)
}
func (m *Message) XXX_Size() int {
	return m.Size()
}
func (m *Message) XXX_DiscardUnknown() {
	xxx_messageInfo_Message.DiscardUnknown(m)
}

var xxx_messageInfo_Message proto.InternalMessageInfo

func (m *Message) GetTerm() uint64 {
	if m != nil {
		return m.Term
	}
	return 0
}

func (m *Message) GetFrom() uint64 {
	if m != nil {
		return m.From
	}
	return 0
}

func (m *Message) GetTo() uint64 {
	if m != nil {
		return m.To
	}
	return 0
}

func (m *Message) GetType() MessageType {
	if m != nil {
		return m.Type
	}
	return MsgUnknown
}

func (m *Message) GetIndex() uint64 {
	if m != nil {
		return m.Index
	}
	return 0
}

func (m *Message) GetLogTerm() uint64 {
	if m != nil {
		return m.LogTerm
	}
	return 0
}

func (m *Message) GetCommit() uint64 {
	if m != nil {
		return m.Commit
	}
	return 0
}

func (m *Message) GetTid() int64 {
	if m != nil {
		return m.Tid
	}
	return 0
}

func (m *Message) GetProxy() uint64 {
	if m != nil {
		return m.Proxy
	}
	return 0
}

func (m *Message) GetEntries() []Entry {
	if m != nil {
		return m.Entries
	}
	return nil
}

func (m *Message) GetSuccess() bool {
	if m != nil {
		return m.Success
	}
	return false
}

type Entry struct {
	Index uint64 `protobuf:"varint,1,opt,name=index" json:"index"`
	Term  uint64 `protobuf:"varint,2,opt,name=term" json:"term"`
	Data  []byte `protobuf:"bytes,3,opt,name=data" json:"data"`
}

func (m *Entry) Reset()         { *m = Entry{} }
func (m *Entry) String() string { return proto.CompactTextString(m) }
func (*Entry) ProtoMessage()    {}
func (*Entry) Descriptor() ([]byte, []int) {
	return fileDescriptor_b042552c306ae59b, []int{1}
}
func (m *Entry) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *Entry) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_Entry.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *Entry) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Entry.Merge(m, src)
}
func (m *Entry) XXX_Size() int {
	return m.Size()
}
func (m *Entry) XXX_DiscardUnknown() {
	xxx_messageInfo_Entry.DiscardUnknown(m)
}

var xxx_messageInfo_Entry proto.InternalMessageInfo

func (m *Entry) GetIndex() uint64 {
	if m != nil {
		return m.Index
	}
	return 0
}

func (m *Entry) GetTerm() uint64 {
	if m != nil {
		return m.Term
	}
	return 0
}

func (m *Entry) GetData() []byte {
	if m != nil {
		return m.Data
	}
	return nil
}

type Empty struct {
}

func (m *Empty) Reset()         { *m = Empty{} }
func (m *Empty) String() string { return proto.CompactTextString(m) }
func (*Empty) ProtoMessage()    {}
func (*Empty) Descriptor() ([]byte, []int) {
	return fileDescriptor_b042552c306ae59b, []int{2}
}
func (m *Empty) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *Empty) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_Empty.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *Empty) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Empty.Merge(m, src)
}
func (m *Empty) XXX_Size() int {
	return m.Size()
}
func (m *Empty) XXX_DiscardUnknown() {
	xxx_messageInfo_Empty.DiscardUnknown(m)
}

var xxx_messageInfo_Empty proto.InternalMessageInfo

func init() {
	proto.RegisterEnum("pb.MessageType", MessageType_name, MessageType_value)
	proto.RegisterType((*Message)(nil), "pb.Message")
	proto.RegisterType((*Entry)(nil), "pb.Entry")
	proto.RegisterType((*Empty)(nil), "pb.Empty")
}

func init() { proto.RegisterFile("raft.proto", fileDescriptor_b042552c306ae59b) }

var fileDescriptor_b042552c306ae59b = []byte{
	// 451 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x6c, 0x92, 0xcf, 0x6b, 0xdb, 0x30,
	0x14, 0xc7, 0x2d, 0xff, 0x88, 0xd3, 0xe7, 0xd2, 0x1a, 0x31, 0x86, 0x08, 0xc5, 0x33, 0x39, 0x39,
	0x83, 0xa5, 0x90, 0xdb, 0x8e, 0xed, 0xe8, 0x31, 0x50, 0x42, 0xb7, 0x9d, 0x1d, 0x47, 0xf1, 0xcc,
	0x6a, 0x4b, 0x58, 0x32, 0xab, 0xff, 0x8b, 0xc1, 0xfe, 0xa9, 0x1c, 0x73, 0xdb, 0x4e, 0x63, 0x4b,
	0xfe, 0x91, 0x21, 0x45, 0xde, 0x04, 0xeb, 0x4d, 0xef, 0xf3, 0x41, 0x7a, 0xef, 0x7d, 0x11, 0x40,
	0x9b, 0x6f, 0xe5, 0x9c, 0xb7, 0x4c, 0x32, 0xec, 0xf2, 0xf5, 0xe4, 0x4d, 0x59, 0xc9, 0x4f, 0xdd,
	0x7a, 0x5e, 0xb0, 0xfa, 0xba, 0x64, 0x25, 0xbb, 0xd6, 0x6a, 0xdd, 0x6d, 0x75, 0xa5, 0x0b, 0x7d,
	0x3a, 0x5d, 0x99, 0x7e, 0x77, 0x21, 0x5c, 0x52, 0x21, 0xf2, 0x92, 0x62, 0x02, 0xbe, 0xa4, 0x6d,
	0x4d, 0x50, 0x8a, 0x32, 0xff, 0xd6, 0xdf, 0xfd, 0x7c, 0xe5, 0xac, 0x34, 0x51, 0x66, 0xdb, 0xb2,
	0x9a, 0xb8, 0xb6, 0x51, 0x04, 0xbf, 0x00, 0x57, 0x32, 0xe2, 0x59, 0xdc, 0x95, 0x0c, 0xcf, 0xc0,
	0x97, 0x3d, 0xa7, 0xc4, 0x4f, 0x51, 0x76, 0xb1, 0xb8, 0x9c, 0xf3, 0xf5, 0xdc, 0x34, 0x79, 0xe8,
	0x39, 0xfd, 0xfb, 0x74, 0xcf, 0x29, 0x9e, 0x40, 0x50, 0x35, 0x1b, 0xfa, 0x44, 0x02, 0xeb, 0x8d,
	0x13, 0xc2, 0x09, 0x84, 0x8f, 0xac, 0x7c, 0x50, 0x33, 0x8d, 0x2c, 0x3b, 0x40, 0x7c, 0x05, 0xa3,
	0x82, 0xd5, 0x75, 0x25, 0x49, 0x68, 0x69, 0xc3, 0xf0, 0x4b, 0xf0, 0x64, 0xb5, 0x21, 0xe3, 0x14,
	0x65, 0x9e, 0x51, 0x0a, 0xa8, 0x8e, 0xbc, 0x65, 0x4f, 0x3d, 0x39, 0xb3, 0x3b, 0x6a, 0x84, 0x67,
	0x10, 0xd2, 0x46, 0xb6, 0x15, 0x15, 0x04, 0x52, 0x2f, 0x8b, 0x16, 0x67, 0x6a, 0xf6, 0xbb, 0x46,
	0xb6, 0xfd, 0xd0, 0xdc, 0x78, 0x35, 0x9c, 0xe8, 0x8a, 0x82, 0x0a, 0x41, 0xa2, 0x14, 0x65, 0xe3,
	0xc1, 0x1b, 0x38, 0xfd, 0x08, 0x81, 0xbe, 0xf7, 0x6f, 0x43, 0xf4, 0xff, 0x86, 0x43, 0xe4, 0xee,
	0x73, 0x91, 0x6f, 0x72, 0x99, 0xeb, 0x68, 0xcf, 0x07, 0xa3, 0xc8, 0x34, 0x84, 0xe0, 0xae, 0xe6,
	0xb2, 0x7f, 0xfd, 0x0d, 0x41, 0x64, 0xc5, 0x8a, 0x2f, 0x00, 0x96, 0xa2, 0x7c, 0xdf, 0x7c, 0x6e,
	0xd8, 0x97, 0x26, 0x76, 0x30, 0xc0, 0x68, 0x29, 0xca, 0x1b, 0xce, 0x63, 0x64, 0xdc, 0x0d, 0xe7,
	0x2b, 0x2a, 0x78, 0xec, 0xe2, 0x08, 0xc2, 0xa5, 0x28, 0x57, 0x34, 0xdf, 0xc4, 0x1e, 0xbe, 0x84,
	0xc8, 0x14, 0xda, 0xfa, 0xc6, 0xde, 0xb7, 0x8c, 0xc7, 0x81, 0xb1, 0xaa, 0xd0, 0x76, 0x64, 0xec,
	0x07, 0x26, 0x69, 0x1c, 0x1a, 0xab, 0x0a, 0x6d, 0xc7, 0x8b, 0xb7, 0x70, 0xbe, 0xca, 0xb7, 0xf2,
	0x5e, 0x7d, 0xaf, 0x82, 0x3d, 0xe2, 0x19, 0x44, 0xef, 0x58, 0x5d, 0x77, 0x4d, 0x55, 0xe4, 0x92,
	0xe2, 0xc8, 0xfa, 0x0c, 0x93, 0x53, 0xba, 0x6a, 0x99, 0xa9, 0x93, 0xa1, 0xdb, 0xab, 0xfd, 0xef,
	0xc4, 0xd9, 0x1d, 0x12, 0xb4, 0x3f, 0x24, 0xe8, 0xd7, 0x21, 0x41, 0x5f, 0x8f, 0x89, 0xb3, 0x3f,
	0x26, 0xce, 0x8f, 0x63, 0xe2, 0xfc, 0x09, 0x00, 0x00, 0xff, 0xff, 0x77, 0x02, 0xbb, 0xf1, 0xea,
	0x02, 0x00, 0x00,
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// RaftProtocolClient is the client API for RaftProtocol service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type RaftProtocolClient interface {
	Communicate(ctx context.Context, opts ...grpc.CallOption) (RaftProtocol_CommunicateClient, error)
}

type raftProtocolClient struct {
	cc *grpc.ClientConn
}

func NewRaftProtocolClient(cc *grpc.ClientConn) RaftProtocolClient {
	return &raftProtocolClient{cc}
}

func (c *raftProtocolClient) Communicate(ctx context.Context, opts ...grpc.CallOption) (RaftProtocol_CommunicateClient, error) {
	stream, err := c.cc.NewStream(ctx, &_RaftProtocol_serviceDesc.Streams[0], "/pb.RaftProtocol/Communicate", opts...)
	if err != nil {
		return nil, err
	}
	x := &raftProtocolCommunicateClient{stream}
	return x, nil
}

type RaftProtocol_CommunicateClient interface {
	Send(*Message) error
	CloseAndRecv() (*Empty, error)
	grpc.ClientStream
}

type raftProtocolCommunicateClient struct {
	grpc.ClientStream
}

func (x *raftProtocolCommunicateClient) Send(m *Message) error {
	return x.ClientStream.SendMsg(m)
}

func (x *raftProtocolCommunicateClient) CloseAndRecv() (*Empty, error) {
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	m := new(Empty)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// RaftProtocolServer is the server API for RaftProtocol service.
type RaftProtocolServer interface {
	Communicate(RaftProtocol_CommunicateServer) error
}

// UnimplementedRaftProtocolServer can be embedded to have forward compatible implementations.
type UnimplementedRaftProtocolServer struct {
}

func (*UnimplementedRaftProtocolServer) Communicate(srv RaftProtocol_CommunicateServer) error {
	return status.Errorf(codes.Unimplemented, "method Communicate not implemented")
}

func RegisterRaftProtocolServer(s *grpc.Server, srv RaftProtocolServer) {
	s.RegisterService(&_RaftProtocol_serviceDesc, srv)
}

func _RaftProtocol_Communicate_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(RaftProtocolServer).Communicate(&raftProtocolCommunicateServer{stream})
}

type RaftProtocol_CommunicateServer interface {
	SendAndClose(*Empty) error
	Recv() (*Message, error)
	grpc.ServerStream
}

type raftProtocolCommunicateServer struct {
	grpc.ServerStream
}

func (x *raftProtocolCommunicateServer) SendAndClose(m *Empty) error {
	return x.ServerStream.SendMsg(m)
}

func (x *raftProtocolCommunicateServer) Recv() (*Message, error) {
	m := new(Message)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

var _RaftProtocol_serviceDesc = grpc.ServiceDesc{
	ServiceName: "pb.RaftProtocol",
	HandlerType: (*RaftProtocolServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Communicate",
			Handler:       _RaftProtocol_Communicate_Handler,
			ClientStreams: true,
		},
	},
	Metadata: "raft.proto",
}

func (m *Message) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Message) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *Message) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	i--
	if m.Success {
		dAtA[i] = 1
	} else {
		dAtA[i] = 0
	}
	i--
	dAtA[i] = 0x58
	if len(m.Entries) > 0 {
		for iNdEx := len(m.Entries) - 1; iNdEx >= 0; iNdEx-- {
			{
				size, err := m.Entries[iNdEx].MarshalToSizedBuffer(dAtA[:i])
				if err != nil {
					return 0, err
				}
				i -= size
				i = encodeVarintRaft(dAtA, i, uint64(size))
			}
			i--
			dAtA[i] = 0x52
		}
	}
	i = encodeVarintRaft(dAtA, i, uint64(m.Proxy))
	i--
	dAtA[i] = 0x48
	i = encodeVarintRaft(dAtA, i, uint64(m.Tid))
	i--
	dAtA[i] = 0x40
	i = encodeVarintRaft(dAtA, i, uint64(m.Commit))
	i--
	dAtA[i] = 0x38
	i = encodeVarintRaft(dAtA, i, uint64(m.LogTerm))
	i--
	dAtA[i] = 0x30
	i = encodeVarintRaft(dAtA, i, uint64(m.Index))
	i--
	dAtA[i] = 0x28
	i = encodeVarintRaft(dAtA, i, uint64(m.Type))
	i--
	dAtA[i] = 0x20
	i = encodeVarintRaft(dAtA, i, uint64(m.To))
	i--
	dAtA[i] = 0x18
	i = encodeVarintRaft(dAtA, i, uint64(m.From))
	i--
	dAtA[i] = 0x10
	i = encodeVarintRaft(dAtA, i, uint64(m.Term))
	i--
	dAtA[i] = 0x8
	return len(dAtA) - i, nil
}

func (m *Entry) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Entry) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *Entry) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.Data != nil {
		i -= len(m.Data)
		copy(dAtA[i:], m.Data)
		i = encodeVarintRaft(dAtA, i, uint64(len(m.Data)))
		i--
		dAtA[i] = 0x1a
	}
	i = encodeVarintRaft(dAtA, i, uint64(m.Term))
	i--
	dAtA[i] = 0x10
	i = encodeVarintRaft(dAtA, i, uint64(m.Index))
	i--
	dAtA[i] = 0x8
	return len(dAtA) - i, nil
}

func (m *Empty) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Empty) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *Empty) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	return len(dAtA) - i, nil
}

func encodeVarintRaft(dAtA []byte, offset int, v uint64) int {
	offset -= sovRaft(v)
	base := offset
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return base
}
func (m *Message) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	n += 1 + sovRaft(uint64(m.Term))
	n += 1 + sovRaft(uint64(m.From))
	n += 1 + sovRaft(uint64(m.To))
	n += 1 + sovRaft(uint64(m.Type))
	n += 1 + sovRaft(uint64(m.Index))
	n += 1 + sovRaft(uint64(m.LogTerm))
	n += 1 + sovRaft(uint64(m.Commit))
	n += 1 + sovRaft(uint64(m.Tid))
	n += 1 + sovRaft(uint64(m.Proxy))
	if len(m.Entries) > 0 {
		for _, e := range m.Entries {
			l = e.Size()
			n += 1 + l + sovRaft(uint64(l))
		}
	}
	n += 2
	return n
}

func (m *Entry) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	n += 1 + sovRaft(uint64(m.Index))
	n += 1 + sovRaft(uint64(m.Term))
	if m.Data != nil {
		l = len(m.Data)
		n += 1 + l + sovRaft(uint64(l))
	}
	return n
}

func (m *Empty) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	return n
}

func sovRaft(x uint64) (n int) {
	return (math_bits.Len64(x|1) + 6) / 7
}
func sozRaft(x uint64) (n int) {
	return sovRaft(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *Message) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowRaft
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: Message: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Message: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Term", wireType)
			}
			m.Term = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowRaft
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Term |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 2:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field From", wireType)
			}
			m.From = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowRaft
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.From |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 3:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field To", wireType)
			}
			m.To = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowRaft
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.To |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 4:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Type", wireType)
			}
			m.Type = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowRaft
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Type |= MessageType(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 5:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Index", wireType)
			}
			m.Index = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowRaft
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Index |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 6:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field LogTerm", wireType)
			}
			m.LogTerm = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowRaft
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.LogTerm |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 7:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Commit", wireType)
			}
			m.Commit = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowRaft
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Commit |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 8:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Tid", wireType)
			}
			m.Tid = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowRaft
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Tid |= int64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 9:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Proxy", wireType)
			}
			m.Proxy = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowRaft
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Proxy |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 10:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Entries", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowRaft
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthRaft
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthRaft
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Entries = append(m.Entries, Entry{})
			if err := m.Entries[len(m.Entries)-1].Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 11:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Success", wireType)
			}
			var v int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowRaft
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.Success = bool(v != 0)
		default:
			iNdEx = preIndex
			skippy, err := skipRaft(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthRaft
			}
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthRaft
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *Entry) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowRaft
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: Entry: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Entry: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Index", wireType)
			}
			m.Index = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowRaft
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Index |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 2:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Term", wireType)
			}
			m.Term = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowRaft
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Term |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 3:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Data", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowRaft
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				byteLen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if byteLen < 0 {
				return ErrInvalidLengthRaft
			}
			postIndex := iNdEx + byteLen
			if postIndex < 0 {
				return ErrInvalidLengthRaft
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Data = append(m.Data[:0], dAtA[iNdEx:postIndex]...)
			if m.Data == nil {
				m.Data = []byte{}
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipRaft(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthRaft
			}
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthRaft
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *Empty) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowRaft
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: Empty: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Empty: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		default:
			iNdEx = preIndex
			skippy, err := skipRaft(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthRaft
			}
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthRaft
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func skipRaft(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	depth := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowRaft
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		wireType := int(wire & 0x7)
		switch wireType {
		case 0:
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowRaft
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if dAtA[iNdEx-1] < 0x80 {
					break
				}
			}
		case 1:
			iNdEx += 8
		case 2:
			var length int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowRaft
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				length |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if length < 0 {
				return 0, ErrInvalidLengthRaft
			}
			iNdEx += length
		case 3:
			depth++
		case 4:
			if depth == 0 {
				return 0, ErrUnexpectedEndOfGroupRaft
			}
			depth--
		case 5:
			iNdEx += 4
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
		if iNdEx < 0 {
			return 0, ErrInvalidLengthRaft
		}
		if depth == 0 {
			return iNdEx, nil
		}
	}
	return 0, io.ErrUnexpectedEOF
}

var (
	ErrInvalidLengthRaft        = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowRaft          = fmt.Errorf("proto: integer overflow")
	ErrUnexpectedEndOfGroupRaft = fmt.Errorf("proto: unexpected end of group")
)
