package fcodec

import (
	"bytes"
	"encoding/binary"
	"errors"
	"github.com/panjf2000/gnet/v2"
	"hash/crc32"
	"io"
)

// 采用大端序
type frameCodec struct {
}

func NewFrameCodec() FrameCodec {
	return frameCodec{}
}

func (fc frameCodec) Encode(fr Frame) ([]byte, error) {
	// Step 1: 构造 payload 部分
	payloadLen := 1 + ReqIdSize + len(fr.Payload.Body)
	if payloadLen > MaxPayloadSize {
		return nil, ErrFrameTooLarge
	}

	// 分配 payload buffer
	payloadBuf := make([]byte, payloadLen)
	payloadBuf[0] = byte(fr.Payload.PayloadProtocol)
	copy(payloadBuf[1:1+ReqIdSize], fr.Payload.ReqId[:])
	if len(fr.Payload.Body) > 0 {
		copy(payloadBuf[1+ReqIdSize:], fr.Payload.Body)
	}

	// Step 2: 计算 checksum (CRC32 over payloadBuf)
	checksum := crc32.ChecksumIEEE(payloadBuf)

	// Step 3: 分配完整帧 buffer
	totalLen := HeaderFrameSize + payloadLen // 12 + payloadLen
	frameBuf := make([]byte, totalLen)

	// Step 4: 填充帧头（大端序）
	binary.BigEndian.PutUint16(frameBuf[0:2], MagicNumber)
	frameBuf[2] = Version
	frameBuf[3] = uint8(fr.Header.Ftype)
	binary.BigEndian.PutUint32(frameBuf[4:8], uint32(payloadLen))
	binary.BigEndian.PutUint32(frameBuf[8:12], checksum)

	// Step 5: 填充 payload
	copy(frameBuf[HeaderFrameSize:], payloadBuf)

	return frameBuf, nil
}

func (fc frameCodec) Decode(c gnet.Conn) (Frame, error) {
	var fr Frame

	// Step 1: Peek 帧头 (12 bytes)
	buf, err := c.Peek(HeaderFrameSize)
	if err != nil {
		if errors.Is(err, io.ErrShortBuffer) {
			return fr, ErrIncompleteFrame
		}

		return fr, err
	}

	// Step 2: 校验 Magic
	if !bytes.Equal(magicBytes, buf[:2]) {
		return fr, ErrInvalidMagic
	}

	// Step 3: 校验 Version
	if buf[2] != Version {
		return fr, ErrVersionUnSupport
	}

	ftype := buf[3]
	payloadLen := binary.BigEndian.Uint32(buf[4:8])
	checksum := binary.BigEndian.Uint32(buf[8:12])

	// Step 4: 长度校验
	if payloadLen > MaxPayloadSize {
		return fr, ErrFrameTooLarge
	}

	totalLen := HeaderFrameSize + int(payloadLen)
	if payloadLen == 0 {
		// 无 payload，但需校验 checksum（应为 0）
		if checksum != 0 {
			return fr, ErrChecksumFailed
		}
		// 消费帧头
		_, _ = c.Discard(HeaderFrameSize)
		fr.Header = FrameHeader{
			Magic:      MagicNumber,
			Version:    Version,
			Ftype:      FrameType(ftype),
			PayloadLen: 0,
			CheckSum:   0,
		}
		return fr, nil
	}

	// Step 5: Peek 完整帧（含 payload）
	fullBuf, err := c.Peek(totalLen)
	if err != nil {
		if errors.Is(err, io.ErrShortBuffer) {
			return fr, ErrIncompleteFrame
		}
		return fr, err
	}

	// Step 6: 提取 payload 部分（用于 checksum 和解析）
	payloadBuf := fullBuf[HeaderFrameSize:]

	// Step 7: 校验 checksum（覆盖整个 payloadBuf）
	if checksum != crc32.ChecksumIEEE(payloadBuf) {
		return fr, ErrChecksumFailed
	}

	// Step 8: 解析 payload 结构
	if len(payloadBuf) < 1+ReqIdSize { // PayloadProtocol(1B) + ReqId(32B)
		return fr, ErrInvalidFrame
	}

	payloadCopy := make([]byte, len(payloadBuf))
	copy(payloadCopy, payloadBuf)

	payloadProtocol := PayloadProtocolType(payloadCopy[0])

	var reqId [ReqIdSize]byte
	copy(reqId[:], payloadCopy[1:1+ReqIdSize])

	body := payloadCopy[1+ReqIdSize:]

	// Step 9: 消费数据
	_, _ = c.Discard(totalLen)

	// Step 10: 组装返回
	fr.Header = FrameHeader{
		Magic:      MagicNumber,
		Version:    Version,
		Ftype:      FrameType(ftype),
		PayloadLen: payloadLen,
		CheckSum:   checksum,
	}
	fr.Payload = Payload{
		PayloadProtocol: payloadProtocol,
		ReqId:           reqId,
		Body:            body,
	}

	return fr, nil
}
