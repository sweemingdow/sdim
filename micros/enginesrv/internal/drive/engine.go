package drive

import (
	"context"
	"errors"
	"fmt"
	"github.com/panjf2000/gnet/v2"
	"github.com/sweemingdow/gmicro_pkg/pkg/lifetime"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/codec/fcodec"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/core"
	"time"
)

type ConnDriveEngine interface {
	lifetime.LifeCycle

	gnet.EventHandler

	Run() error
}

type connDriveEngine struct {
	eng       gnet.Engine
	addr      string
	multicore bool
	batchRead int
	frCodec   fcodec.FrameCodec
	frHandler core.FrameHandler
	connMgr   core.ConnManager
}

func NewConnDriveEngine(
	port int,
	multicore bool,
	batchRead int,
	frCodec fcodec.FrameCodec,
	frHandler core.FrameHandler,
	connMgr core.ConnManager,
) ConnDriveEngine {
	return &connDriveEngine{
		addr:      fmt.Sprintf("tcp://:%d", port),
		multicore: multicore,
		batchRead: batchRead,
		frCodec:   frCodec,
		frHandler: frHandler,
		connMgr:   connMgr,
	}
}

func (cde *connDriveEngine) OnCreated(_ chan<- error) {
}

// OnBoot fires when the engine is ready for accepting connections.
// The parameter engine has information and various utilities.
func (cde *connDriveEngine) OnBoot(en gnet.Engine) (action gnet.Action) {
	cde.eng = en

	lg := mylog.AppLogger()
	lg.Info().Msg("connection drive engin on boot")

	return
}

// OnShutdown fires when the engine is being shut down, it is called right after
// all event-loops and connections are closed.
func (cde *connDriveEngine) OnShutdown(en gnet.Engine) {
	lg := mylog.AppLogger()
	lg.Info().Msg("connection drive engin on shutdown")
}

// OnOpen fires when a new connection has been opened.
// The parameter out is the return value which is going to be sent back to the remote.
func (cde *connDriveEngine) OnOpen(c gnet.Conn) (out []byte, action gnet.Action) {
	ccCtx := core.BindConnCtxWhenOpen(c)
	lg := core.LoggerWithCcCtx(ccCtx)
	lg.Debug().Msg("a new connection was opened")

	cde.connMgr.AddWhenOpened(core.ConnAddParam{
		ConnId: ccCtx.Id,
		Conn:   c,
	})

	return
}

// OnClose fires when a connection has been closed.
// The parameter err is the last known connection error.
func (cde *connDriveEngine) OnClose(c gnet.Conn, err error) (action gnet.Action) {
	ccCtx := core.GetConnCtx(c)
	lg := core.LoggerWithCcCtx(ccCtx)
	if err != nil {
		lg.Error().Stack().Err(err).Msg("a connection was closed, but has a error")
	} else {
		lg.Info().Msg("a connection was closed")
	}

	cde.connMgr.CleanAfterConnClosed(ccCtx.Id)

	return
}

// OnTraffic fires when a local socket receives data from the remote.
func (cde *connDriveEngine) OnTraffic(c gnet.Conn) (action gnet.Action) {
	ccCtx := core.GetConnCtx(c)
	lg := core.LoggerWithCcCtx(ccCtx)

	lg.Debug().Msg("connection drive engin on traffic")

	var frs []fcodec.Frame
	for i := 0; i < cde.batchRead; i++ {
		frame, err := cde.frCodec.Decode(c)
		// 帧数据未满
		if err == fcodec.ErrIncompleteFrame {
			break
		}

		if err != nil {
			lg.Error().Stack().Err(err).Msg("decode frame data failed, will be close")
			return gnet.Close
		}

		frs = append(frs, frame)
	}

	if len(frs) == 0 {
		return gnet.None
	}

	// 连接是否认证
	connAuthed, exists := cde.connMgr.ConnHadAuthed(ccCtx.Id)
	if !exists {
		// 连接都不存在, 直接关闭
		lg.Warn().Stack().Msg("connection not exists, close it directly")
		return gnet.Close
	}

	// 异步批量处理
	action = cde.frHandler.Handle(connAuthed, c, frs)
	if action != gnet.None {
		return action
	}

	// 兜底唤醒
	if len(frs) == cde.batchRead && c.InboundBuffered() > 0 {
		if err := c.Wake(nil); err != nil {
			lg.Error().Stack().Err(err).Msg("connection wake failed, will be close")
			return gnet.Close
		}
	}

	return gnet.None
}

// OnTick fires immediately after the engine starts and will fire again
// following the duration specified by the delay return value.
func (cde *connDriveEngine) OnTick() (delay time.Duration, action gnet.Action) {
	lg := mylog.AppLogger()
	lg.Info().Msg("connection drive engin on tick")

	return
}

func (cde *connDriveEngine) OnDispose(ctx context.Context) error {
	var errs []error
	err := cde.eng.Stop(ctx)
	if err != nil {
		errs = append(errs, err)
	}

	err = cde.connMgr.GracefulStop(ctx)
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

func (cde *connDriveEngine) Run() error {
	return gnet.Run(
		cde,
		cde.addr,
		gnet.WithMulticore(cde.multicore),
	)
}
