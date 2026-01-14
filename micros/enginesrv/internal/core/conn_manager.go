package core

import (
	"fmt"
	"github.com/panjf2000/gnet/v2"
	"github.com/rs/zerolog"
	"github.com/sweemingdow/gmicro_pkg/pkg/graceful"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils"
	"github.com/sweemingdow/sdim/pkg/constt"
)

type ConnCtx struct {
	Id string // 连接的id
}

var emptyCtx = &ConnCtx{}

func (cc ConnCtx) IsEmpty() bool {
	return cc.Id == ""
}

// just call In gnet.EventHandler
func BindConnCtxWhenOpen(c gnet.Conn) *ConnCtx {
	ccCtx := &ConnCtx{
		Id: fmt.Sprintf("%d-%s", c.Fd(), utils.RandStr(8)),
	}

	c.SetContext(ccCtx)

	return ccCtx
}

func GetConnCtx(c gnet.Conn) *ConnCtx {
	ctxVal := c.Context()
	if ctx, ok := ctxVal.(*ConnCtx); ok {
		return ctx
	}

	return emptyCtx
}

func GetConnId(c gnet.Conn) string {
	return GetConnCtx(c).Id
}

func LoggerWithCcCtx(ccCtx *ConnCtx) zerolog.Logger {
	return mylog.AppLogger().
		With().
		Str("conn_id", ccCtx.Id).
		Logger()
}

func LoggerWithGnetConn(c gnet.Conn) zerolog.Logger {
	ccCtx := GetConnCtx(c)
	return mylog.AppLogger().
		With().
		Str("conn_id", ccCtx.Id).
		Logger()
}

type ConnAddParam struct {
	ConnId string
	Conn   gnet.Conn
}

type ConnModifyParam struct {
	ConnId string
	Uid    string
	CType  constt.ClientType
}

type ConnHandleItem struct {
	ConnId  string
	Silence bool
}

// 连接管理器, 管理所有连接
type ConnManager interface {
	graceful.Gracefully

	// 连接连上时调用
	AddWhenOpened(pa ConnAddParam)

	// 已认证调用
	ModifyAfterAuthed(pa ConnModifyParam) []ConnHandleItem

	// 连接是否认证过
	ConnHadAuthed(connId string) (bool, bool)

	// gnet.EventHandler回调时调用
	CleanAfterConnClosed(connId string)

	//客户端ping成功调用
	ModifyAfterPingSuccess(connId string)

	// 获取用户的所有连接
	GetUserConns(uid string) []gnet.Conn

	// 获取用户的所有连接
	GetUsersConns(uids []string) map[string][]gnet.Conn

	// more actions...
}
