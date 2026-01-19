package fcodec

import (
	"encoding/binary"
	"errors"
	"github.com/google/uuid"
	"github.com/panjf2000/gnet/v2"
)

var (
	ErrInvalidFrame     = errors.New("invalid frame")
	ErrFrameTooLarge    = errors.New("frame too large")
	ErrIncompleteFrame  = errors.New("incomplete frame")
	ErrInvalidMagic     = errors.New("invalid magic number")
	ErrVersionUnSupport = errors.New("version not support")
	ErrChecksumFailed   = errors.New("checksum verification failed")
)

// 数据帧类型
type FrameType uint8

const (
	Ping       FrameType = 1  // ping(c2s)
	Pong       FrameType = 2  // pong(s2c)
	Conn       FrameType = 3  // 连接(c2s)
	ConnAck    FrameType = 4  // 连接确认(s2c)
	Send       FrameType = 5  // 发送消息(c2s)
	SendAck    FrameType = 6  // 发送消息确认(s2c)
	Forward    FrameType = 7  // 转发消息(s2c)
	ForwardAck FrameType = 8  // 收到转发消息确认(c2s)
	ConvUpdate FrameType = 9  // 收到转发消息确认(c2s)
	Disconnect FrameType = 10 // 断开连接(服务端主动发起)
)

var (
	ft2desc = map[FrameType]string{
		Ping:       "Ping",
		Pong:       "ping",
		Conn:       "Conn",
		ConnAck:    "ConnAck",
		Send:       "Send",
		SendAck:    "SendAck",
		Forward:    "Forward",
		ForwardAck: "ForwardAck",
		Disconnect: "Disconnect",
	}
)

func GetFrameTypeDesc(ft FrameType) string {
	return ft2desc[ft]
}

// 荷载协议类型(json, protobuf, thrift...)
type PayloadProtocolType uint8

const (
	JsonPayload     PayloadProtocolType = 1
	MsgpackPayload  PayloadProtocolType = 2
	ProtobufPayload PayloadProtocolType = 3
	ThriftPayload   PayloadProtocolType = 4
)

const (
	HeaderFrameSize = 2 + 1 + 1 + 4 + 4 // 帧头数据长度
	MaxPayloadSize  = 1 << 20           // 荷载最大长度 1MB
	MagicNumber     = 0xCAFE            // 模数验证
	Version         = 1                 // 版本
	ReqIdSize       = 16
)

var (
	magicBytes = func() []byte {
		b := make([]byte, 2)
		binary.BigEndian.PutUint16(b, MagicNumber)
		return b
	}()

	pongBytes = []byte{'p', 'o', 'n', 'g'}

	connAckBytes = []byte{'c', 'o', 'n', 'n', 'a', 'c', 'k'}
)

var (
	emptyPayload = Payload{}
)

type (
	Frame struct {
		Header  FrameHeader // 帧头
		Payload Payload     // 帧荷载
	}

	FrameHeader struct {
		Magic      uint16    // 模数 2b
		Version    uint8     // 版本 1b
		Ftype      FrameType // 帧类型 1b
		PayloadLen uint32    // 荷载长度 4b
		CheckSum   uint32    // 校验和 4b
	}

	Payload struct {
		PayloadProtocol PayloadProtocolType // 荷载协议 1b
		ReqId           [ReqIdSize]byte     // 请求id(标准的uuid) 16b
		Body            []byte              // 数据 nb
	}
)

func NewPongFrame(pl Payload) Frame {
	return newS2cFrame(pl.PayloadProtocol, Pong, pl.ReqId, pongBytes)
}

func NewS2cFrame(from Payload, ft FrameType, body any) (Frame, error) {
	pl, err := EncodePayload(from, body)
	if err != nil {
		var fr Frame
		return fr, err
	}

	frh := FrameHeader{
		Magic:      MagicNumber,
		Version:    Version,
		Ftype:      ft,
		PayloadLen: uint32(1 + ReqIdSize + len(pl.Body)),
		CheckSum:   0,
	}

	return Frame{
		Header:  frh,
		Payload: pl,
	}, nil
}

func NewForwardFrame(reqId string, bodies []byte) (Frame, error) {
	frh := FrameHeader{
		Magic:      MagicNumber,
		Version:    Version,
		Ftype:      Forward,
		PayloadLen: uint32(1 + ReqIdSize + len(bodies)),
		CheckSum:   0,
	}

	uuReqId, err := uuid.Parse(reqId)
	if err != nil {
		uuReqId = [ReqIdSize]byte{}
	}

	return Frame{
		Header: frh,
		Payload: Payload{
			PayloadProtocol: JsonPayload, // todo
			ReqId:           uuReqId,
			Body:            bodies,
		},
	}, nil
}

func NewConvUpdateFrame(bodies []byte) Frame {
	return newS2cFrame(JsonPayload, ConvUpdate, [16]byte{}, bodies)
}

func newS2cFrame(pp PayloadProtocolType, ft FrameType, reqId [16]byte, body []byte) Frame {
	frh := FrameHeader{
		Magic:      MagicNumber,
		Version:    Version,
		Ftype:      ft,
		PayloadLen: uint32(1 + ReqIdSize + len(body)),
		CheckSum:   0,
	}

	return Frame{
		Header: frh,
		Payload: Payload{
			PayloadProtocol: pp,
			ReqId:           reqId,
			Body:            body,
		},
	}
}

// 传输层数据编解码
type FrameCodec interface {
	// 将payload编码为帧数据
	Encode(fr Frame) ([]byte, error)

	// 解码帧数据
	Decode(c gnet.Conn) (Frame, error)
}
