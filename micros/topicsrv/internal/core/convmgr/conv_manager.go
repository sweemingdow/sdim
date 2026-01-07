package convmgr

import (
	"context"
	"errors"
	"fmt"
	"github.com/nsqio/go-nsq"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/cid/sfid"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/cnsq"
	"github.com/sweemingdow/gmicro_pkg/pkg/guc"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/parser/json"
	"github.com/sweemingdow/gmicro_pkg/pkg/server/srpc/rpccall"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils"
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
	uid2convItems map[string]*core.MemberConvWrap
	convSegLock   *guc.SegmentRwLock[string]
	uidSegLock    *guc.SegmentRwLock[string]
	done          chan struct{}
	closed        atomic.Bool
	cr            convrepo.ConvRepository
	nsdPd         *cnsq.NsqProducer
	doneChan      chan *nsq.ProducerTransaction
	userProvider  rpcuser.UserInfoRpcProvider
}

func NewConvManager(esCap, strip int,
	cr convrepo.ConvRepository,
	nsdPd *cnsq.NsqProducer,
	userProvider rpcuser.UserInfoRpcProvider,
) core.ConvManager {
	cm := &convManager{
		//convId2items:  haxmap.New[string, *core.Conversation](uintptr(esCap)),
		convId2items:  make(map[string]*core.Conversation, esCap),
		uid2convItems: make(map[string]*core.MemberConvWrap, esCap),
		convSegLock:   guc.NewSegmentRwLock[string](strip, nil),
		uidSegLock:    guc.NewSegmentRwLock[string](strip, nil),
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

type convHandleResult struct {
	convItems []*convrepo.ConvItem
	conv      *core.Conversation
	convTree  map[string]string
}

func (cm *convManager) OnMsgComing(pa core.MsgComingParam) core.MsgComingResult {
	ve := msgmodel.ValidateMsgContent(pa.MsgContent)

	if ve != nil {
		return core.MsgComingResult{
			Err: ve,
		}
	}

	convId := generateConvId(pa)
	var mcr = core.MsgComingResult{
		MsgId:  sfid.Next(),
		ConvId: convId,
	}

	convType := chatconst.GetConvTypeWithChatType(pa.ChatType)
	if pa.IsConvType {
		if convType == chatconst.P2pConv { // 单聊
			if err := cm.p2pConvHandle(convId, mcr.MsgId, convType, pa); err != nil {
				mcr.Err = err
				return mcr
			}
		}
	} else { // Danmaku

	}

	return mcr
}

func (cm *convManager) OnMsgStored(pd *msgpd.MsgSendReceivePayload) {
	//TODO implement me
	panic("implement me")
}

func (cm *convManager) p2pConvHandle(convId string, msgId int64, convType chatconst.ConvType, pa core.MsgComingParam) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Second)
	defer cancel()

	lg := mylog.AppLogger()

	// 处理会话
	chrVal, convErr := cm.convSegLock.WithLock(
		convId,
		func(lock *sync.RWMutex) (any, error) {
			lock.RLock()
			c, ok := cm.convId2items[convId]
			lock.RUnlock()

			if ok {
				return convHandleResult{
					conv: c,
				}, nil
			}

			lock.Lock()
			defer lock.Unlock()

			c, ok = cm.convId2items[convId]

			if ok {
				return convHandleResult{
					conv: c,
				}, nil
			}

			// db查询会话记录
			conv, err := cm.cr.FindConvItems(ctx, convId)
			if err != nil {
				return convHandleResult{}, err
			}

			// 会话记录存在
			if conv != nil {
				c = cm.dbConv2local(conv)
				lg.Trace().Str("conv_id", convId).Msg("conversion was exist in db")

				cm.convId2items[convId] = c

				uids := usli.Conv(conv.Items, func(item *convrepo.ConvItem) string {
					return item.OwnerUid
				})
				// 从redis捞取convTree信息
				convTree, err := cm.cr.GetConvTree(ctx, convId, uids)
				if err != nil {
					return convHandleResult{}, err
				}

				lg.Trace().Str("conv_id", convId).Msgf("found convTree, convTree:%+v", convTree)

				var (
					lastMsgId    int64
					msgSeq       int64
					lastActiveTs int64
				)

				val := convTree[convrepo.LastMsgIdKey]
				if val != "" {
					lastMsgId = utils.MustParseI64(val)
				}
				val = convTree[convrepo.MsgSeqKey]
				if val != "" {
					msgSeq = utils.MustParseI64(val)
				}
				val = convTree[convrepo.LastActiveTs]
				if val != "" {
					lastActiveTs = utils.MustParseI64(val)
				}

				c.LastActiveTs = lastActiveTs
				c.MsgSeq = msgSeq
				c.LastMsgId = lastMsgId

				return convHandleResult{
					conv:      c,
					convItems: conv.Items,
				}, nil
				// to do
			} else { // 记录不存在
				lg.Debug().Str("conv_id", convId).Msg("conversion was not exist in db, create and save now")

				c = &core.Conversation{
					Id:      convId,
					Type:    convType,
					Members: []string{pa.Sender, pa.Receiver},
				}

				req := rpccall.CreateIdReq(pa.ReqId, rpcuser.UsersUnitInfoReq{Uids: c.Members})
				resp, err := cm.userProvider.UsersUnitInfo(req)
				if err != nil {
					return convHandleResult{}, err
				}

				infoMap, err := resp.OkOrErr()
				if err != nil {
					return convHandleResult{}, err
				}

				if len(infoMap) != len(c.Members) {
					return convHandleResult{}, fmt.Errorf("user unit info results incorrect, size not eq, may users not exists, uids:%v", c.Members)
				}

				infoSli := umap.ToSli(infoMap)
				// 存db

				conv = cm.localConv2db(c, infoSli)
				if err = cm.cr.UpsertConv(ctx, conv); err != nil {
					return convHandleResult{}, err
				}

				cm.convId2items[convId] = c

				return convHandleResult{
					conv:      c,
					convItems: conv.Items,
				}, nil
			}
		})

	if convErr != nil {
		return convErr
	}

	chr := chrVal.(convHandleResult)

	// 处理用户会话
	if len(chr.convItems) > 0 {
		var (
			browseSeq int64
			msgSeq    int64
		)

		convTree := chr.convTree
		val := convTree[convrepo.MsgSeqKey]
		if val != "" {
			msgSeq = utils.MustParseI64(val)
		}

		for _, convItem := range chr.convItems {
			browseSeqVal := convTree[convrepo.PkgBrowseSeqKey(convItem.OwnerUid)]
			if browseSeqVal != "" {
				browseSeq = utils.MustParseI64(browseSeqVal)
			}

			if browseSeq == 0 {
				browseSeq = convItem.BrowseMsgSeq
			}

			unreadCnt := msgSeq - browseSeq
			if unreadCnt < 0 {
				unreadCnt = 0
			}

			uid := convItem.OwnerUid
			_, _ = cm.uidSegLock.WithLock(uid, func(lock *sync.RWMutex) (any, error) {
				lock.RLock()
				ci, ok := cm.uid2convItems[uid]
				lock.RUnlock()

				if ok {
					lock.Lock()
					// 更新部分属性
					if mebConv, ok := ci.ConvId2Item[convId]; ok {
						mebConv.Title = convItem.Title
						mebConv.Icon = convItem.Icon
						mebConv.BrowseMsgSeq = browseSeq
						mebConv.UnreadCount = unreadCnt
					}

					lock.Unlock()
					return nil, nil
				}

				lock.Lock()
				defer lock.Unlock()

				ci, ok = cm.uid2convItems[uid]

				if ok {
					if mebConv, ok := ci.ConvId2Item[convId]; ok {
						mebConv.Title = convItem.Title
						mebConv.Icon = convItem.Icon
						mebConv.BrowseMsgSeq = browseSeq
						mebConv.UnreadCount = unreadCnt
					}
					return nil, nil
				}

				ci = &core.MemberConvWrap{
					ConvId2Item: make(map[string]*core.MemberConv, 100),
					ConvItems:   make([]*core.MemberConv, 0, 100),
				}

				cm.uid2convItems[uid] = ci

				mebConv := &core.MemberConv{
					Id:           convId,
					Type:         convType,
					Icon:         convItem.Icon,
					Title:        convItem.Title,
					RelationId:   convItem.RelationId,
					Remark:       convItem.Remark,
					PinTop:       convItem.PinTop,
					NoDisturb:    convItem.NoDisturb,
					BrowseMsgSeq: browseSeq,
					UnreadCount:  unreadCnt,
					Cts:          convItem.Cts,
					Uts:          convItem.Uts,
				}

				ci.ConvId2Item[convId] = mebConv
				ci.ConvItems = append(ci.ConvItems, mebConv)

				return nil, nil
			})
		}
	}

	seq, lastTs, err := cm.cr.ModifyConvTree(context.Background(), pa.Sender, convId, msgId)

	if err != nil {
		return err
	}

	// exclude self
	mebUid, ok := usli.FindFirstIf(chr.conv.Members, func(val string) bool {
		return val != pa.Sender
	})

	if !ok {
		return errors.New("can not found receiver in conv:" + convId)
	}

	err = cm.sendMsgWithMq(&msgpd.MsgSendReceivePayload{
		ConvId:           convId,
		ConvLastActiveTs: lastTs,
		MsgId:            msgId,
		ClientUniqueId:   pa.ClientUniqueId,
		MsgSeq:           seq,
		ChatType:         pa.ChatType,
		Sender:           pa.Sender,
		Receiver:         pa.Receiver,
		Members:          []string{mebUid},
		ReqId:            pa.ReqId,
		SendTs:           time.Now().UnixMilli(),
		MsgContent:       pa.MsgContent,
	})

	if err != nil {
		return err
	}

	return nil
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
			OwnerUid:   meb,
			RelationId: info.Uid,
			Icon:       icon,
			Title:      title,
			Cts:        conv.Cts,
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

			//cm.convSegLock.PureRead(convId, func() {
			//	if conv, ok := cm.convId2items[convId]; ok {
			//		// 设置消息序列号自增
			//		//conv.SetMsgSeq(msgSeq)
			//	}
			//})
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
