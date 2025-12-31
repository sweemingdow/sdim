package core

import (
	"github.com/panjf2000/gnet/v2"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/codec/fcodec"
)

type FrameHandler interface {
	// 处理一个连接的多帧数据
	Handle(connAuthed bool, c gnet.Conn, frs []fcodec.Frame) gnet.Action
}
