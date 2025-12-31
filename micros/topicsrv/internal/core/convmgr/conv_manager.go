package convmgr

import (
	"context"
	"errors"
	"fmt"
	"github.com/alphadose/haxmap"
	"github.com/nsqio/go-nsq"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/cid/sfid"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/cnsq"
	"github.com/sweemingdow/gmicro_pkg/pkg/guc"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/parser/json"
	"github.com/sweemingdow/gmicro_pkg/pkg/server/srpc/rpccall"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils/umap"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils/usli"
	"github.com/sweemingdow/sdim/external/eglobal/chatconst"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst/payload/msgpd"
	"github.com/sweemingdow/sdim/external/emodel/msgmodel"
	"github.com/sweemingdow/sdim/external/erpc/rpcuser"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/core"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/repostories/convrepo"
	"sync"
	"sync/atomic"
	"time"
)

func init() {
	sfid.Setting(false)
}

type convManager struct {
	// 会话id -> 会话
	//convId2items *haxmap.Map[string, *core.Conversation]

	convId2items map[string]*core.Conversation

	// uid -> convWrap
	// 用户所有的会话
	uid2convItems *haxmap.Map[string, *core.MemberConvWrap]

	segLock      *guc.SegmentRwLock[string]
	done         chan struct{}
	closed       atomic.Bool
	cr           convrepo.ConvRepository
	nsdPd        *cnsq.NsqProducer
	doneChan     chan *nsq.ProducerTransaction
	userProvider rpcuser.UserInfoRpcProvider
}

func NewConvManager(esCap, strip int,
	cr convrepo.ConvRepository,
	nsdPd *cnsq.NsqProducer,
	userProvider rpcuser.UserInfoRpcProvider,
) core.ConvManager {
	cm := &convManager{
		//convId2items:  haxmap.New[string, *core.Conversation](uintptr(esCap)),
		convId2items:  make(map[string]*core.Conversation, esCap),
		uid2convItems: haxmap.New[string, *core.MemberConvWrap](uintptr(esCap)),
		segLock:       guc.NewSegmentRwLock[string](strip, nil),
		done:          make(chan struct{}),
		cr:            cr,
		userProvider:  userProvider,
		nsdPd:         nsdPd,
		doneChan:      make(chan *nsq.ProducerTransaction, 10),
	}

	go cm.receiveMqSendAsyncResult()

	return cm
}

func (cm *convManager) GracefulStop(ctx context.Context) error {
	if !cm.closed.CompareAndSwap(false, true) {
		return nil
	}

	close(cm.done)

	return nil
}

func (cm *convManager) OnMsgComing(pa core.MsgComingParam) core.MsgComingResult {
	convId := generateConvId(pa)
	var mcr = core.MsgComingResult{
		MsgId:  sfid.Next(),
		ConvId: convId,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()

	lg := mylog.AppLogger()
	convType := chatconst.GetConvTypeWithChatType(pa.ChatType)
	if pa.IsConvType {
		if convType == chatconst.P2pConv { // 单聊
			cn, err := cm.segLock.WithLock(convId, func(lock *sync.RWMutex) (any, error) {
				lock.RLock()
				c, ok := cm.convId2items[convId]
				lock.RUnlock()

				if ok {
					return c, nil
				}

				lock.Lock()
				defer lock.Unlock()

				c, ok = cm.convId2items[convId]

				if ok {
					return c, nil
				}

				// db查询会话记录
				conv, err := cm.cr.FindConvItems(ctx, convId)
				if err != nil {
					return nil, err
				}

				// 会话记录存在
				if conv != nil {
					c = cm.dbConv2local(conv)
					lg.Trace().Str("conv_id", convId).Msg("conversion was exist in db")

					cm.convId2items[convId] = c

					return c, nil
					// to do
				} else { // 记录不存在
					lg.Debug().Str("conv_id", convId).Msg("conversion was not exist in db, create and save now")

					c = &core.Conversation{
						Id:      convId,
						Type:    convType,
						Members: []string{pa.Sender, pa.Receiver},
					}

					c.ResetMsgSeq(0)

					req := rpccall.CreateIdReq(pa.ReqId, rpcuser.UsersUnitInfoReq{Uids: c.Members})
					resp, e := cm.userProvider.UsersUnitInfo(req)
					if e != nil {
						return nil, e
					}

					infoMap, e := resp.OkOrErr()
					if e != nil {
						return nil, e
					}

					if len(infoMap) != len(c.Members) {
						return nil, fmt.Errorf("user unit info results incorrect, size not eq, may users not exists, uids:%v", c.Members)
					}

					infoSli := umap.ToSli(infoMap)
					// 存db
					if e = cm.cr.UpsertConv(ctx, cm.localConv2db(c, infoSli)); e != nil {
						return nil, e
					}

					cm.convId2items[convId] = c

					// todo 触发事件, 会话创建
					return c, nil
				}
			})

			if err != nil {
				mcr.Err = err
				return mcr
			}

			mc, err := msgmodel.ParseMsgContent(pa.MsgBody)

			if err != nil {
				mcr.Err = err
				return mcr
			}

			// 自增
			_ctx, _cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer _cancel()

			seq, err := cm.cr.ModifyConvTree(_ctx, pa.Sender, convId, mcr.MsgId)

			if err != nil {
				mcr.Err = err
				return mcr
			}

			conv := cn.(*core.Conversation)

			// exclude self
			mebUid, ok := usli.FindFirstIf(conv.Members, func(val string) bool {
				return val != pa.Sender
			})
			if !ok {
				err = errors.New("can not found receiver in conv:" + convId)
				mcr.Err = err
				return mcr
			}

			err = cm.sendMsgWithMq(&msgpd.MsgSendReceivePayload{
				ConvId:     convId,
				MsgId:      mcr.MsgId,
				MsgSeq:     seq,
				ChatType:   pa.ChatType,
				Sender:     pa.Sender,
				Receiver:   pa.Receiver,
				Members:    []string{mebUid},
				ReqId:      pa.ReqId,
				SendTs:     time.Now().UnixMilli(),
				MsgContent: mc,
			})

			if err != nil {
				mcr.Err = err
				return mcr
			}
		}

	} else { // Danmaku

	}

	return mcr
}

func (cm *convManager) sendMsgWithMq(pd *msgpd.MsgSendReceivePayload) error {
	b, err := json.Fmt(pd)
	if err != nil {
		return err
	}
	// 拿到会话, mq异步移到msg_srv
	err = cm.nsdPd.PublishAsync(cnsq.PublishParam{
		Topic:   nsqconst.MsgReceiveTopic,
		Payload: b,
	}, cm.doneChan, []any{pd.ConvId, pd.MsgSeq})

	return nil
}

func (cm *convManager) dbConv2local(conv *convrepo.Conv) *core.Conversation {
	c := &core.Conversation{
		Id:   conv.ConvId,
		Type: chatconst.ConvType(conv.Items[0].ConvType),
		Members: usli.Conv(conv.Items, func(item *convrepo.ConvItem) string {
			return item.OwnerUid
		}),
	}

	c.ResetMsgSeq(uint64(conv.MsgSeq))

	return c
}

func (cm *convManager) localConv2db(c *core.Conversation, infoSli []*rpcuser.UnitInfoRespItem) *convrepo.Conv {
	conv := &convrepo.Conv{
		ConvId:   c.Id,
		Cts:      time.Now().UnixMilli(),
		ConvType: int8(c.Type),
		Items:    make([]*convrepo.ConvItem, len(c.Members)),
	}

	for i, meb := range c.Members {
		info, _ := usli.FindFirstIf(infoSli, func(val *rpcuser.UnitInfoRespItem) bool {
			// 找对方
			return val.Uid != meb
		})

		icon := info.Avatar
		title := info.Nickname

		conv.Items[i] = &convrepo.ConvItem{
			OwnerUid:    meb,
			RelationUid: info.Uid,
			Icon:        icon,
			Title:       title,
			Cts:         conv.Cts,
		}
	}

	return conv
}

func (cm *convManager) receiveMqSendAsyncResult() {
	for {
		select {
		case <-cm.done:
			return
		case pt, ok := <-cm.doneChan:
			if !ok {
				return
			}

			convId, ok := pt.Args[0].(string)
			if !ok {
				continue
			}

			msgSeq, ok := pt.Args[1].(int64)
			if !ok {
				continue
			}

			lg := mylog.AppLogger()
			if pt.Error != nil {
				lg.Error().Stack().Str("conv_id", convId).Int64("msg_seq", msgSeq).Err(pt.Error).Msg("mq msg async send failed")
				continue
			}

			lg.Trace().Str("conv_id", convId).Int64("msg_seq", msgSeq).Msg("mq msg async send success")

			cm.segLock.PureRead(convId, func() {
				if conv, ok := cm.convId2items[convId]; ok {
					// 设置消息序列号自增
					conv.SetMsgSeq(msgSeq)
				}
			})
		}
	}
}

func generateConvId(pa core.MsgComingParam) string {
	convType := chatconst.GetConvTypeWithChatType(pa.ChatType)
	if convType == chatconst.P2pConv {
		if pa.Sender <= pa.Receiver {
			return fmt.Sprintf("p2p:%s:%s", pa.Sender, pa.Receiver)
		}

		return fmt.Sprintf("p2p:%s:%s", pa.Receiver, pa.Sender)
	} else if convType == chatconst.GroupConv {
		return "grp:" + pa.Receiver
	} else if convType == chatconst.CustomerConv {
		return "cus:" + pa.Receiver
	} else if pa.ChatType == chatconst.Danmaku {

	}

	return ""
}
