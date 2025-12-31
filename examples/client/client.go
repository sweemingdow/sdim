package client

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"net"
	"sync"
	"time"
)

const (
	HeaderFrameSize = 12 // 2+1+1+4+4
	ReqIdSize       = 16
	MagicNumber     = 0xCAFE
	Version         = 1
	MaxPayloadSize  = 1 << 20 // 1MB

	// 心跳间隔：根据服务器要求调整，这里设为 15 秒
	HeartbeatInterval = 120 * time.Second
	// 读取超时：用于检测连接僵死，应大于心跳间隔
	ReadTimeout = 30 * time.Second
)

// FrameType 数据帧类型
type FrameType uint8

const (
	Ping       FrameType = 1
	Pong       FrameType = 2
	Conn       FrameType = 3
	ConnAck    FrameType = 4
	Send       FrameType = 5
	SendAck    FrameType = 6
	Recv       FrameType = 7
	RecvAck    FrameType = 8
	Disconnect FrameType = 9
)

// PayloadProtocolType 荷载协议类型
type PayloadProtocolType uint8

const (
	JsonPayload PayloadProtocolType = 1
)

// FrameHeader 帧头
type FrameHeader struct {
	Magic      uint16
	Version    uint8
	Ftype      FrameType
	PayloadLen uint32
	CheckSum   uint32
}

// Payload 帧荷载
type Payload struct {
	PayloadProtocol PayloadProtocolType
	ReqId           [ReqIdSize]byte
	Body            []byte
}

// Frame 完整帧
type Frame struct {
	Header  FrameHeader
	Payload Payload
}

// ConnFrameBody Conn帧的body结构
type ConnFrameBody struct {
	Uid     string `json:"uid,omitempty"`
	CType   int    `json:"ctype,omitempty"`
	TsMills int64  `json:"tsMills,omitempty"`
}

// ConnAckFrameBody ConnAck帧的body结构
type ConnAckFrameBody struct {
	TimeDiff int64 `json:"timeDiff"`
}

type (
	SendFrameAck struct {
		ErrCode int32            `json:"errCode,omitempty"`
		ErrDesc string           `json:"errDesc,omitempty"`
		Data    SendFrameAckBody `json:"data,omitempty"`
	}

	SendFrameAckBody struct {
		MsgId  int64  `json:"msgId,omitempty"`  // 消息id
		ConvId string `json:"convId,omitempty"` // 会话id
	}
)

// Client 客户端
type Client struct {
	conn            net.Conn
	addr            string
	uid             string
	cType           int
	connected       bool
	mu              sync.RWMutex
	stopChan        chan struct{}
	once            sync.Once // 确保 Close 只执行一次
	readWg          sync.WaitGroup
	frameChan       chan *Frame
	readCancel      context.CancelFunc
	heartbeatCtx    context.Context
	heartbeatCancel context.CancelFunc
}

// NewClient 创建新的客户端
func NewClient(addr string, uid string, cType int) *Client {
	return &Client{
		addr:      addr,
		uid:       uid,
		cType:     cType,
		connected: false,
		stopChan:  make(chan struct{}),
		frameChan: make(chan *Frame, 100),
	}
}

// Connect 连接到服务器，并在1秒内等待ConnAck，否则超时断连
func (c *Client) Connect() error {
	conn, err := net.Dial("tcp", c.addr)
	if err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	c.conn = conn

	// 发送 conn 帧
	err = c.sendConnFrame()
	if err != nil {
		c.conn.Close()
		return fmt.Errorf("send conn frame failed: %w", err)
	}

	// 👇 关键：设置读取超时为1秒
	c.conn.SetReadDeadline(time.Now().Add(1 * time.Second))

	// 读取 conn_ack 帧（同步读，带超时）
	frame, err := c.readFrame()
	if err != nil {
		c.conn.Close()
		// 判断是否是超时错误
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return fmt.Errorf("timeout waiting for ConnAck (1s)")
		}
		return fmt.Errorf("read conn ack failed: %w", err)
	}

	// 恢复读取 deadline（可选，因后续由 readLoop 控制）
	c.conn.SetReadDeadline(time.Time{})

	if frame.Header.Ftype != ConnAck {
		c.conn.Close()
		return fmt.Errorf("unexpected frame type: %d, expected ConnAck", frame.Header.Ftype)
	}

	// 解析 conn_ack
	var connAckBody ConnAckFrameBody
	err = json.Unmarshal(frame.Payload.Body, &connAckBody)
	if err != nil {
		c.conn.Close()
		return fmt.Errorf("parse conn ack body failed: %w", err)
	}

	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()

	return nil
}

// sendConnFrame 发送连接帧
func (c *Client) sendConnFrame() error {
	var reqId [ReqIdSize]byte
	_, err := rand.Read(reqId[:])
	if err != nil {
		return fmt.Errorf("generate reqId failed: %w", err)
	}

	connBody := ConnFrameBody{
		Uid:     c.uid,
		CType:   c.cType,
		TsMills: time.Now().UnixMilli(),
	}

	bodyBytes, err := json.Marshal(connBody)
	if err != nil {
		return fmt.Errorf("marshal conn body failed: %w", err)
	}

	payload := Payload{
		PayloadProtocol: JsonPayload,
		ReqId:           reqId,
		Body:            bodyBytes,
	}

	frame := Frame{
		Header: FrameHeader{
			Magic:      MagicNumber,
			Version:    Version,
			Ftype:      Conn,
			PayloadLen: uint32(1 + ReqIdSize + len(bodyBytes)),
			CheckSum:   0,
		},
		Payload: payload,
	}

	frameBytes, err := c.encodeFrame(frame)
	if err != nil {
		return fmt.Errorf("encode frame failed: %w", err)
	}

	_, err = c.conn.Write(frameBytes)
	return err
}

// StartReadLoop 启动后台读循环（必须在 Connect 成功后调用）
func (c *Client) StartReadLoop() error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	c.mu.RUnlock()

	// 防止重复启动
	c.mu.Lock()
	if c.readCancel != nil {
		c.mu.Unlock()
		return fmt.Errorf("read loop already started")
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.readCancel = cancel
	c.mu.Unlock()

	// 启动心跳
	c.heartbeatCtx, c.heartbeatCancel = context.WithCancel(context.Background())
	c.readWg.Add(1)
	go c.heartbeatLoop()

	c.readWg.Add(1)
	go c.readLoop(ctx)

	return nil
}

func (c *Client) heartbeatLoop() {
	defer c.readWg.Done()
	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.heartbeatCtx.Done():
			return
		case <-c.stopChan:
			return
		case <-ticker.C:
			// 发送前检查连接是否还有效
			c.mu.RLock()
			connected := c.connected
			c.mu.RUnlock()
			if !connected {
				return
			}
			if err := c.SendPing(); err != nil {
				c.triggerDisconnect()
				return
			}
		}
	}
}

// readLoop 异步读取循环（无短超时）
func (c *Client) readLoop(ctx context.Context) {
	defer c.readWg.Done()

	for {
		// 先检查是否已取消
		select {
		case <-ctx.Done():
			return
		case <-c.stopChan:
			return
		default:
		}

		// 设置 deadline（即使刚被取消，也最多等 ReadTimeout）
		c.conn.SetReadDeadline(time.Now().Add(ReadTimeout))

		frame, err := c.readFrame()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// 超时后继续循环，下一轮会检查 ctx
				continue
			}
			c.triggerDisconnect()
			return
		}

		// 处理帧
		if frame.Header.Ftype == Disconnect {
			c.mu.Lock()
			c.connected = false
			c.mu.Unlock()
		}

		select {
		case c.frameChan <- &frame:
		case <-ctx.Done():
			return
		case <-c.stopChan:
			return
		}
	}
}

// triggerDisconnect 触发连接断开（由客户端检测到异常时调用）
// todo revert
func (c *Client) triggerDisconnect() {
	//c.mu.Lock()
	//wasConnected := c.connected
	//c.connected = false
	//c.mu.Unlock()
	//
	//if wasConnected {
	//	// 通知上层：底层连接已断（非协议 Disconnect）
	//	select {
	//	case c.frameChan <- &Frame{
	//		Header: FrameHeader{
	//			Ftype: Disconnect,
	//		},
	//	}:
	//	default:
	//	}
	//}
}

// SendPing 发送 ping 帧
func (c *Client) SendPing() error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	c.mu.RUnlock()

	var reqId [ReqIdSize]byte
	_, err := rand.Read(reqId[:])
	if err != nil {
		return fmt.Errorf("generate reqId failed: %w", err)
	}

	payload := Payload{
		PayloadProtocol: JsonPayload,
		ReqId:           reqId,
		Body:            []byte("ping"),
	}

	frame := Frame{
		Header: FrameHeader{
			Magic:      MagicNumber,
			Version:    Version,
			Ftype:      Ping,
			PayloadLen: uint32(1 + ReqIdSize + len(payload.Body)),
			CheckSum:   0,
		},
		Payload: payload,
	}

	frameBytes, err := c.encodeFrame(frame)
	if err != nil {
		return fmt.Errorf("encode ping frame failed: %w", err)
	}

	_, err = c.conn.Write(frameBytes)
	if err != nil {
		c.triggerDisconnect()
		return fmt.Errorf("send ping failed: %w", err)
	}

	return nil
}

// SendMessage 发送消息帧
func (c *Client) SendMessage(message string) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	c.mu.RUnlock()

	var reqId [ReqIdSize]byte
	_, err := rand.Read(reqId[:])
	if err != nil {
		return fmt.Errorf("generate reqId failed: %w", err)
	}

	msgBody := map[string]interface{}{
		"message": message,
		"time":    time.Now().UnixMilli(),
	}

	bodyBytes, err := json.Marshal(msgBody)
	if err != nil {
		return fmt.Errorf("marshal message failed: %w", err)
	}

	payload := Payload{
		PayloadProtocol: JsonPayload,
		ReqId:           reqId,
		Body:            bodyBytes,
	}

	frame := Frame{
		Header: FrameHeader{
			Magic:      MagicNumber,
			Version:    Version,
			Ftype:      Send,
			PayloadLen: uint32(1 + ReqIdSize + len(bodyBytes)),
			CheckSum:   0,
		},
		Payload: payload,
	}

	frameBytes, err := c.encodeFrame(frame)
	if err != nil {
		return fmt.Errorf("encode message frame failed: %w", err)
	}

	_, err = c.conn.Write(frameBytes)
	if err != nil {
		c.triggerDisconnect()
		return fmt.Errorf("send message failed: %w", err)
	}

	return nil
}

type (
	ChatType uint8

	SendFrameBody struct {
		Sender    string   `json:"sender,omitempty"`   // 发送者uid
		Receiver  string   `json:"receiver,omitempty"` // 接收者, 单聊是对方的uid, 群聊是群id
		ChatType  ChatType `json:"chatType,omitempty"`
		SendMills int64    `json:"sendMills,omitempty"`
		Sign      string   `json:"sign,omitempty"` // 消息签名, 防纂改
		Ttl       int32    `json:"ttl,omitempty"`  // 消息过期时间(sec), -1:阅后即焚,0:不过期
		MsgBody   any      `json:"msgBody,omitempty"`
	}
)

const (
	P2pChat      ChatType = 1 // 单聊
	GroupChat    ChatType = 2 // 群聊
	CustomerChat ChatType = 3 // 客服
)

// SendJSON 发送JSON数据
func (c *Client) SendJSON(data interface{}) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected")
	}
	c.mu.RUnlock()

	var reqId [ReqIdSize]byte
	_, err := rand.Read(reqId[:])
	if err != nil {
		return fmt.Errorf("generate reqId failed: %w", err)
	}

	bodyBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal JSON failed: %w", err)
	}

	payload := Payload{
		PayloadProtocol: JsonPayload,
		ReqId:           reqId,
		Body:            bodyBytes,
	}

	frame := Frame{
		Header: FrameHeader{
			Magic:      MagicNumber,
			Version:    Version,
			Ftype:      Send,
			PayloadLen: uint32(1 + ReqIdSize + len(bodyBytes)),
			CheckSum:   0,
		},
		Payload: payload,
	}

	frameBytes, err := c.encodeFrame(frame)
	if err != nil {
		return fmt.Errorf("encode JSON frame failed: %w", err)
	}

	_, err = c.conn.Write(frameBytes)
	if err != nil {
		c.triggerDisconnect()
		return fmt.Errorf("send JSON failed: %w", err)
	}

	return nil
}

// GetFrameChan 获取帧通道（只读）
func (c *Client) GetFrameChan() <-chan *Frame {
	return c.frameChan
}

// IsConnected 检查是否连接
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// Close 安全关闭客户端
func (c *Client) Close() {
	c.once.Do(func() {
		close(c.stopChan)

		if c.readCancel != nil {
			c.readCancel()
		}
		if c.heartbeatCancel != nil {
			c.heartbeatCancel()
		}

		c.mu.Lock()
		if c.conn != nil {
			c.conn.Close()
		}
		c.connected = false
		c.mu.Unlock()

		c.readWg.Wait()
		close(c.frameChan)
	})
}

// ============ 以下为帧编解码逻辑 ============

func (c *Client) readFrame() (Frame, error) {
	var frame Frame
	headerBuf := make([]byte, HeaderFrameSize)

	bytesRead := 0
	for bytesRead < HeaderFrameSize {
		n, err := c.conn.Read(headerBuf[bytesRead:])
		if err != nil {
			return frame, fmt.Errorf("read header failed at byte %d: %w", bytesRead, err)
		}
		bytesRead += n
	}

	frame.Header.Magic = binary.BigEndian.Uint16(headerBuf[0:2])
	frame.Header.Version = headerBuf[2]
	frame.Header.Ftype = FrameType(headerBuf[3])
	frame.Header.PayloadLen = binary.BigEndian.Uint32(headerBuf[4:8])
	frame.Header.CheckSum = binary.BigEndian.Uint32(headerBuf[8:12])

	if frame.Header.Magic != MagicNumber {
		return frame, fmt.Errorf("invalid magic: 0x%04X", frame.Header.Magic)
	}
	if frame.Header.Version != Version {
		return frame, fmt.Errorf("unsupported version: %d", frame.Header.Version)
	}

	if frame.Header.PayloadLen == 0 {
		return frame, nil
	}
	if frame.Header.PayloadLen > MaxPayloadSize {
		return frame, fmt.Errorf("payload too large: %d", frame.Header.PayloadLen)
	}

	payloadBuf := make([]byte, frame.Header.PayloadLen)
	bytesRead = 0
	for bytesRead < int(frame.Header.PayloadLen) {
		n, err := c.conn.Read(payloadBuf[bytesRead:])
		if err != nil {
			return frame, fmt.Errorf("read payload failed: %w", err)
		}
		bytesRead += n
	}

	calculatedChecksum := crc32.ChecksumIEEE(payloadBuf)
	if frame.Header.CheckSum != calculatedChecksum {
		return frame, fmt.Errorf("checksum mismatch: got 0x%08X, want 0x%08X",
			calculatedChecksum, frame.Header.CheckSum)
	}

	// 解析 payload：所有帧都包含 PayloadProtocol + ReqId + Body
	// 如果服务器对 Ack 帧也按此格式返回，则兼容
	if len(payloadBuf) < 1+ReqIdSize {
		return frame, fmt.Errorf("payload too short: %d", len(payloadBuf))
	}

	frame.Payload.PayloadProtocol = PayloadProtocolType(payloadBuf[0])
	copy(frame.Payload.ReqId[:], payloadBuf[1:1+ReqIdSize])
	if len(payloadBuf) > 1+ReqIdSize {
		frame.Payload.Body = payloadBuf[1+ReqIdSize:]
	}

	if debugMode && len(frame.Payload.Body) > 0 {
		fmt.Printf("[DEBUG READ] MsgContent Body (%d bytes): %s\n",
			len(frame.Payload.Body), string(frame.Payload.Body))
	}

	return frame, nil
}

func (c *Client) encodeFrame(frame Frame) ([]byte, error) {
	payloadLen := 1 + ReqIdSize + len(frame.Payload.Body)
	if payloadLen > MaxPayloadSize {
		return nil, fmt.Errorf("frame too large")
	}

	payloadBuf := make([]byte, payloadLen)
	payloadBuf[0] = byte(frame.Payload.PayloadProtocol)
	copy(payloadBuf[1:1+ReqIdSize], frame.Payload.ReqId[:])
	if len(frame.Payload.Body) > 0 {
		copy(payloadBuf[1+ReqIdSize:], frame.Payload.Body)
	}

	checksum := crc32.ChecksumIEEE(payloadBuf)

	totalLen := HeaderFrameSize + payloadLen
	frameBuf := make([]byte, totalLen)
	binary.BigEndian.PutUint16(frameBuf[0:2], MagicNumber)
	frameBuf[2] = Version
	frameBuf[3] = uint8(frame.Header.Ftype)
	binary.BigEndian.PutUint32(frameBuf[4:8], uint32(payloadLen))
	binary.BigEndian.PutUint32(frameBuf[8:12], checksum)
	copy(frameBuf[HeaderFrameSize:], payloadBuf)

	return frameBuf, nil
}

// ============ 辅助函数 ============

var debugMode = false // 默认关闭调试

func GetFrameTypeDesc(ft FrameType) string {
	descriptions := map[FrameType]string{
		Ping:       "Ping",
		Pong:       "Pong",
		Conn:       "Conn",
		ConnAck:    "ConnAck",
		Send:       "Send",
		SendAck:    "SendAck",
		Recv:       "Recv",
		RecvAck:    "RecvAck",
		Disconnect: "Disconnect",
	}
	if desc, ok := descriptions[ft]; ok {
		return desc
	}
	return fmt.Sprintf("Unknown(%d)", ft)
}

func ParseFrameBody(frame *Frame, v interface{}) error {
	if frame.Payload.PayloadProtocol != JsonPayload {
		return fmt.Errorf("unsupported payload protocol: %d", frame.Payload.PayloadProtocol)
	}
	return json.Unmarshal(frame.Payload.Body, v)
}
