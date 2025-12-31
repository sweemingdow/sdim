package connmgr

import (
	"context"
	"github.com/alphadose/haxmap"
	"github.com/panjf2000/gnet/v2"
	dq "github.com/sweemingdow/delay-queue"
	"github.com/sweemingdow/gmicro_pkg/pkg/guc"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils/usli"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/core"
	"github.com/sweemingdow/sdim/pkg/constt"
	"sync/atomic"
	"time"
)

const (
	authExpired DelayType = 1 // 认证过期检测
	pingExpired DelayType = 2 // ping过期检测
)

type (
	DelayType uint8

	connDelayItem struct {
		delayType DelayType
		connId    string
		expired   time.Time
	}
)

func (cdi connDelayItem) GetExpire() time.Time {
	return cdi.expired
}

type connManager struct {
	// uid -> connWrap
	// 用户认证过的所有连接
	uid2conn *haxmap.Map[string, *clientConnWrap]

	// connId -> conn
	connId2conn *haxmap.Map[string, *clientConn]

	// 分段锁
	segLock *guc.SegmentLock[string]

	// 延时队列
	dq *dq.DelayQueue[connDelayItem]

	// 属于主设备的所有客户端类型, 只读
	masterTypes map[constt.ClientType]struct{}

	closed atomic.Bool

	done chan struct{}

	// 用户最大的从设备连接数
	maxSlaveConns int

	// 连接认证的过期时间, 当客户端连上服务时, 超过这个时间客户端还未发送Conn帧, 则剔除该连接
	// 有效防止恶意空连接和僵尸连接
	authTimeout time.Duration

	// ping 间隔
	pingInterval time.Duration

	// ping丢失次数
	pingLostTimes int
}

func NewConnManager(esCap, strip int,
	maxSlaveConns int,
	masterTypes []constt.ClientType,
	authTimeout, pingInterval time.Duration,
	pingLostTimes int,
) core.ConnManager {
	if esCap < 1000 {
		esCap = 1000
	}

	if strip < 64 {
		strip = 64
	}

	mts := make(map[constt.ClientType]struct{}, len(masterTypes))
	for _, mt := range masterTypes {
		mts[mt] = struct{}{}
	}

	cm := &connManager{
		uid2conn:    haxmap.New[string, *clientConnWrap](uintptr(esCap)),
		connId2conn: haxmap.New[string, *clientConn](uintptr(esCap)),
		segLock:     guc.NewSegmentLock[string](strip, nil),
		dq: dq.NewDelayQueue(
			esCap/10,
			func(o1, o2 connDelayItem) bool {
				return o1.connId == o2.connId && o1.delayType == o2.delayType
			},
		),
		done:          make(chan struct{}),
		masterTypes:   mts,
		maxSlaveConns: maxSlaveConns,
		authTimeout:   authTimeout,
		pingInterval:  pingInterval,
		pingLostTimes: pingLostTimes,
	}

	go cm.startDqTaker()

	return cm
}

func (cm *connManager) GracefulStop(ctx context.Context) error {
	if !cm.closed.CompareAndSwap(false, true) {
		return nil
	}

	close(cm.done)

	return nil
}

func (cm *connManager) AddWhenOpened(pa core.ConnAddParam) {
	cm.connId2conn.Set(
		pa.ConnId,
		newClientConn(pa.ConnId, pa.Conn),
	)

	_ = cm.dq.Offer(connDelayItem{
		delayType: authExpired,
		connId:    pa.ConnId,
		expired:   time.Now().Add(cm.authTimeout),
	})
}

func (cm *connManager) ModifyAfterAuthed(pa core.ConnModifyParam) []core.ConnHandleItem {
	var (
		connId = pa.ConnId
		uid    = pa.Uid
	)
	conn, ok := cm.connId2conn.Get(connId)
	if !ok {
		return nil
	}

	// opened -> authed
	if !conn.state.CompareAndSwap(connStateOpened, connStateAuthed) {
		return nil
	}

	var handleItems []core.ConnHandleItem

	cm.segLock.SafeCall(
		uid,
		func() {
			now := time.Now()
			conn.authedTime = now
			conn.uid = uid
			conn.ctype = pa.CType
			conn.clevel = cm.getClientLeveLWithClientType(pa.CType)
			// 认证通过, 算一次ping
			conn.lastPingTime.Store(now)

			// 将认证过的连接加入到用户的连接中
			connWrap, loaded := cm.uid2conn.GetOrCompute(
				uid,
				func() *clientConnWrap {
					return newClientConnWrap(conn, conn.clevel)
				},
			)

			// 已存在
			if loaded {
				var (
					isMaster  = cm.isMasterLevel(conn.ctype)
					oldMaster *clientConn
					earliest  *clientConn
				)

				// 加入总连接数
				connWrap.conns = append(connWrap.conns, conn)

				if isMaster {
					// 已存在用户的主设备, 需要剔除原来的主设备
					oldConns, exists := connWrap.level2conn[constt.Master]
					if exists && len(oldConns) > 0 {
						oldMaster = oldConns[0]

						connWrap.conns = usli.RemoveFirstIf(connWrap.conns, func(cc *clientConn) bool {
							return cc.connId == oldMaster.connId
						})

						handleItems = append(
							handleItems,
							core.ConnHandleItem{
								ConnId:  oldMaster.connId,
								Silence: false, // 主设备不做静默剔除
							},
						)
					}

					// 存储用户新的主设备
					connWrap.level2conn[constt.Master] = []*clientConn{conn}
				} else {
					oldConns, exists := connWrap.level2conn[constt.Slave]
					connWrap.level2conn[constt.Slave] = append(oldConns, conn)

					if exists {
						// 超过从设备的最大连接数
						if len(connWrap.level2conn[constt.Slave]) > cm.maxSlaveConns {
							// 剔除最早的一台从设备
							earliest = connWrap.level2conn[constt.Slave][0]

							slaveConns := usli.Remove(connWrap.level2conn[constt.Slave], 0)
							connWrap.level2conn[constt.Slave] = slaveConns

							connWrap.conns = usli.RemoveFirstIf(connWrap.conns, func(cc *clientConn) bool {
								return cc.connId == earliest.connId
							})

							handleItems = append(
								handleItems,
								core.ConnHandleItem{
									ConnId:  earliest.connId,
									Silence: true, // 从设备静默剔除
								},
							)
						}
					}
				}
			}
		},
	)

	// 提交检测ping任务
	_ = cm.dq.Offer(connDelayItem{
		delayType: pingExpired,
		connId:    connId,
		expired:   time.Now().Add(cm.pingInterval),
	})

	return handleItems
}

func (cm *connManager) ConnHadAuthed(connId string) bool {
	conn, ok := cm.connId2conn.Get(connId)

	return ok && conn.state.Load() == connStateAuthed
}
func (cm *connManager) CleanAfterConnClosed(connId string) {
	conn, ok := cm.connId2conn.GetAndDel(connId)
	if !ok {
		return
	}

	// 未认证
	connUid := conn.uid
	if connUid == "" {
		return
	}

	connWrap, ok := cm.uid2conn.Get(conn.uid)
	if !ok {
		return
	}

	isMaster := constt.IsMasterLevel(conn.clevel)

	cm.segLock.SafeCall(
		conn.uid,
		func() {
			connWrap.conns = usli.RemoveFirstIf(connWrap.conns, func(val *clientConn) bool {
				return val.connId == connId
			})

			if isMaster {
				connWrap.level2conn[constt.Master] = []*clientConn{}
			} else {
				connWrap.level2conn[constt.Slave] = usli.RemoveFirstIf(connWrap.level2conn[constt.Slave], func(val *clientConn) bool {
					return val.connId == connId
				})
			}

			// 该用户已经没有连接了
			if len(connWrap.conns) == 0 {
				cm.uid2conn.Del(conn.uid)
			}
		},
	)
}

func (cm *connManager) startDqTaker() {
	for {
		select {
		case <-cm.done:
		default:
			item, err := cm.dq.BlockTake()
			if err == dq.QueWasClosedErr {
				return
			}

			if item.delayType == authExpired {
				cm.execAuthExpireCheckTask(item)
			} else if item.delayType == pingExpired {
				cm.execPingExpireCheckTask(item)
			}
		}
	}
}

func (cm *connManager) ModifyAfterPingSuccess(connId string) {
	conn, ok := cm.connId2conn.Get(connId)
	if !ok {
		return
	}

	conn.lastPingTime.Store(time.Now())
}

func (cm *connManager) GetUserConns(uid string) []gnet.Conn {
	connWrap, ok := cm.uid2conn.Get(uid)
	if !ok {
		return []gnet.Conn{}
	}

	var cpConns []*clientConn
	cm.segLock.SafeCall(
		uid,
		func() {
			if len(connWrap.conns) > 0 {
				cpConns = make([]*clientConn, len(connWrap.conns))
				copy(cpConns, connWrap.conns)
			}
		},
	)

	if len(cpConns) > 0 {
		return usli.Conv(cpConns, func(cc *clientConn) gnet.Conn {
			return cc.conn
		})
	}

	return []gnet.Conn{}
}

func (cm *connManager) GetUsersConns(uids []string) map[string][]gnet.Conn {
	if len(uids) == 0 {
		return map[string][]gnet.Conn{}
	}

	m := make(map[string][]gnet.Conn)
	for _, uid := range uids {
		conns := cm.GetUserConns(uid)
		if len(conns) > 0 {
			m[uid] = conns
		}
	}

	return m
}

func (cm *connManager) execAuthExpireCheckTask(item connDelayItem) {
	lg := mylog.AppLogger().With().Str("conn_id", item.connId).Logger()
	lg.Trace().Msg("conn auth expire time reached")

	conn, ok := cm.connId2conn.Get(item.connId)
	if !ok {
		return
	}

	// opened -> closing
	if conn.state.CompareAndSwap(connStateOpened, connStateClosing) {
		lg.Info().Msg("conn auth timeout, kick it")

		// 未认证直接剔除该连接
		_ = conn.close()
		conn.state.Store(connStateClosed)
		cm.connId2conn.Del(item.connId)
	}
}

func (cm *connManager) execPingExpireCheckTask(item connDelayItem) {
	lg := mylog.AppLogger().With().Str("conn_id", item.connId).Logger()
	lg.Trace().Msg("conn ping expire time reached")

	conn, ok := cm.connId2conn.Get(item.connId)
	if !ok {
		return
	}

	if val, ok := conn.lastPingTime.Load().(time.Time); ok {
		// 超过了ping丢失的最大阈值, 需要关闭该连接
		if cm.overPingLostThreshold(val) {
			if conn.state.CompareAndSwap(connStateAuthed, connStateClosing) {
				lg.Info().Msg("conn ping timeout, kick it")

				// 关闭之后会触发: gent.EventHandler 中的 OnClose函数, 该函数会调用 CleanAfterConnClosed 执行清理操作
				_ = conn.close()

				conn.state.Store(connStateClosed)
			}
		} else {
			lg.Trace().Msg("conn ping valid, offer next ping check")
			// ping有效, 提交下一次检查
			_ = cm.dq.Offer(connDelayItem{
				delayType: pingExpired,
				connId:    item.connId,
				expired:   time.Now().Add(cm.pingInterval),
			})
		}
	}
}

func (cm *connManager) overPingLostThreshold(pingTime time.Time) bool {
	return pingTime.Add(cm.pingInterval * time.Duration(cm.pingLostTimes)).Before(time.Now())
}

func (cm *connManager) getClientLeveLWithClientType(ct constt.ClientType) constt.ClientLevel {
	if cm.isMasterLevel(ct) {
		return constt.Master
	}

	return constt.Slave
}

func (cm *connManager) isMasterLevel(ct constt.ClientType) bool {
	_, ok := cm.masterTypes[ct]
	return ok
}

// 连接状态机解决toctou问题
const (
	connStateOpened = iota
	connStateAuthed
	connStateClosing
	connStateClosed
)

// 客户端连接
type clientConn struct {
	connId       string
	uid          string // 连接对应的uid
	conn         gnet.Conn
	ctype        constt.ClientType
	clevel       constt.ClientLevel
	ctime        time.Time
	state        atomic.Uint32 // 连接状态
	authedTime   time.Time     // 认证时间
	lastPingTime atomic.Value  // 上次ping时间
}

func newClientConn(connId string, conn gnet.Conn) *clientConn {
	cc := &clientConn{
		connId: connId,
		conn:   conn,
		ctime:  time.Now(),
	}

	cc.state.Store(connStateOpened)

	return cc
}

func (cc *clientConn) close() error {
	// Close closes the current connection, implements net.Conn, it's concurrency-safe.
	return cc.conn.Close()
}

type clientConnWrap struct {
	// 用户所有的连接
	conns []*clientConn

	// 设备级别 -> 级别下的所有连接, 用于快速查找
	level2conn map[constt.ClientLevel][]*clientConn
}

func newClientConnWrap(cc *clientConn, clevel constt.ClientLevel) *clientConnWrap {
	masterConns := make([]*clientConn, 0, 1)
	slaveConns := make([]*clientConn, 0)

	if clevel == constt.Master {
		masterConns = append(masterConns, cc)
	} else {
		slaveConns = append(slaveConns, cc)
	}

	return &clientConnWrap{
		conns: []*clientConn{cc},
		level2conn: map[constt.ClientLevel][]*clientConn{
			constt.Master: masterConns,
			constt.Slave:  slaveConns,
		},
	}
}
