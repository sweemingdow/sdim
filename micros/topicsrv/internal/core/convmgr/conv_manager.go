package convmgr

import (
	"context"
	"errors"
	"fmt"
	"github.com/gammazero/deque"
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
	"github.com/sweemingdow/sdim/external/erpc/rpcmsg"
	"github.com/sweemingdow/sdim/external/erpc/rpcuser"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/core"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/repostories/convrepo"
	"slices"
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

	convSegLock  *guc.SegmentRwLock[string]
	uidSegLock   *guc.SegmentRwLock[string]
	done         chan struct{}
	closed       atomic.Bool
	cr           convrepo.ConvRepository
	nsdPd        *cnsq.NsqProducer
	doneChan     chan *nsq.ProducerTransaction
	userProvider rpcuser.UserInfoRpcProvider
	msgProvider  rpcmsg.MsgProvider
	msgCap       int
}

func NewConvManager(esCap, strip, msgCap int,
	cr convrepo.ConvRepository,
	nsdPd *cnsq.NsqProducer,
	userProvider rpcuser.UserInfoRpcProvider,
	msgProvider rpcmsg.MsgProvider,
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
		msgProvider:   msgProvider,
		nsdPd:         nsdPd,
		doneChan:      make(chan *nsq.ProducerTransaction, 10),
		msgCap:        msgCap,
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

func (cm *convManager) OnMsgStored(pd *msgpd.MsgForwardPayload) {
	// 更新会话的最新一条消息 & 会话的最近消息
	convId := pd.ConvId

	_, _ = cm.convSegLock.WithLock(
		convId,
		func() (any, error) {
			if conv, ok := cm.convId2items[convId]; ok {
				if pd.MsgId >= conv.LastMsgId {
					conv.LastMsgId = pd.MsgId
					// 更新最后一条消息
					conv.LastMsg = &core.LastMsgInConv{
						MsgId:      pd.MsgId,
						SenderInfo: pd.Msg.SenderInfo,
						Content:    pd.Msg.Content,
					}
				}

				msgQueue := conv.RecentlyMsgs
				msgItem := &msgmodel.MsgItemInMem{
					MsgId:      pd.MsgId,
					ConvId:     convId,
					Sender:     pd.Sender,
					Receiver:   pd.Receiver,
					ChatType:   pd.ChatType,
					MsgType:    pd.Msg.Content.Type,
					Content:    pd.Msg.Content,
					SenderInfo: pd.Msg.SenderInfo,
					MegSeq:     pd.MsgSeq,
					Cts:        pd.Cts,
				}

				msgQueue.PushBack(msgItem)

				// 满了
				for msgQueue.Len() > cm.msgCap {
					msgQueue.PopFront()
				}
			}

			return nil, nil
		})
}

func (cm *convManager) RecentlyConvList(uid string) []*core.ConvListItem {
	items, _ := cm.convList(uid, false)
	return items
}

func (cm *convManager) SyncHotConvList(uid string) []*core.ConvListItem {
	lg := mylog.AppLogger().With().Str("uid", uid).Logger()

	lg.Debug().Msgf("start sync hot conv list")

	var start = time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Second)
	defer cancel()

	convItems, err := cm.cr.FindUserConvItems(ctx, uid)
	if err != nil {
		lg.Error().Stack().Err(err).Msg("find user conv items failed without mem in sync")
		return make([]*core.ConvListItem, 0)
	}
	if len(convItems) == 0 {
		return make([]*core.ConvListItem, 0)
	}

	convIds := usli.Conv(convItems, func(item *convrepo.ConvItem) string {
		return item.ConvId
	})

	// redis中捞会话
	convTrees, err := cm.cr.GetConvTreesWithP2p(ctx, uid, convIds)
	if err != nil {
		lg.Error().Str("uid", uid).Stack().Err(err).Msg("find conv tree failed without mem in sync")
		return make([]*core.ConvListItem, 0)
	}

	// rpc msg_server查会话的最近n条消息
	resp, err := cm.msgProvider.BatchConvRecentlyMsgs(
		rpcmsg.BatchConvRecentlyMsgsReq{
			ConvIds: convIds,
			Limit:   20,
		})
	if err != nil {
		lg.Error().Str("uid", uid).Strs("conv_ids", convIds).Stack().Err(err).Msg("find batch conv recently msgs with rpc failed")
		return make([]*core.ConvListItem, 0)
	}

	convId2msgs := resp.ConvId2Msgs

	var msgSeq, _, lastActiveTs, userBrowseSeq, unreadCount int64
	items := make([]*core.ConvListItem, len(convItems))
	for idx, item := range convItems {
		convTree, ok := convTrees[item.ConvId]
		if ok {
			msgSeq, _, lastActiveTs, userBrowseSeq = convrepo.TransferConvTree(convTree)
		}

		var (
			recentlyMsgs = make([]*msgmodel.MsgItemInMem, 0)
			lastMsg      *core.LastMsgInConv
		)
		msgs, ok := convId2msgs[item.ConvId]
		if ok {
			recentlyMsgs = msgs
			if len(recentlyMsgs) > 0 {
				val := recentlyMsgs[0]
				lastMsg = &core.LastMsgInConv{
					MsgId:      val.MsgId,
					SenderInfo: val.SenderInfo,
					Content:    val.Content,
				}
			}
		}

		items[idx] = &core.ConvListItem{
			ConvId:       item.ConvId,
			ConvType:     chatconst.ConvType(item.ConvType),
			Icon:         item.Icon,
			Title:        item.Title,
			RelationId:   item.RelationId,
			LastMsg:      lastMsg,
			Remark:       item.Remark,
			PinTop:       item.PinTop,
			NoDisturb:    item.NoDisturb,
			MsgSeq:       msgSeq,
			BrowseMsgSeq: userBrowseSeq,
			UnreadCount:  unreadCount,
			Cts:          item.Cts,
			Uts:          lastActiveTs,
			RecentlyMsgs: recentlyMsgs,
		}
	}

	lg.Trace().Dur("took", time.Since(start)).Msg("sync hot conv list completed")

	return items
}

type convListResult struct {
	conv *core.Conversation
	msgs []*msgmodel.MsgItemInMem
	ok   bool
}

func (cm *convManager) convList(uid string, msgs bool) ([]*core.ConvListItem, bool) {
	convItemsVal := cm.uidSegLock.PureReadWithResult(
		uid,
		func() any {
			convWrap, ok := cm.uid2convItems[uid]
			if ok {

				if len(convWrap.ConvItems) > 0 {
					dst := make([]*core.MemberConv, len(convWrap.ConvItems))

					// copy
					copy(dst, convWrap.ConvItems)

					return dst
				}
			}

			return nil
		})

	if nil == convItemsVal {
		return make([]*core.ConvListItem, 0), false
	}

	convItems := convItemsVal.([]*core.MemberConv)

	returnItems := make([]*core.ConvListItem, len(convItems))

	for idx, mebConv := range convItems {
		convId := mebConv.Id
		clrVal := cm.convSegLock.PureReadWithResult(
			convId,
			func() any {
				_conv, ok := cm.convId2items[convId]
				if ok {
					if msgs {
						dq := _conv.RecentlyMsgs
						// copy最近N条消息
						msgItems := make([]*msgmodel.MsgItemInMem, dq.Len())
						for i := range msgItems {
							msgItems[i] = dq.At(i)
						}

						return convListResult{
							conv: _conv,
							msgs: msgItems,
							ok:   true,
						}
					}

					return convListResult{
						conv: _conv,
						ok:   true,
					}

				}

				return convListResult{
					ok: false,
				}
			})

		clr := clrVal.(convListResult)
		if !clr.ok {
			continue
		}
		conv := clr.conv

		var recentlyMsgs []*msgmodel.MsgItemInMem
		if msgs && len(clr.msgs) > 0 {
			// 排序
			slices.SortFunc(clr.msgs, func(a, b *msgmodel.MsgItemInMem) int {
				if a.MsgId > b.MsgId {
					return 1
				} else if a.MsgId < b.MsgId {
					return -1
				}
				return 0
			})

			recentlyMsgs = clr.msgs
		} else {
			recentlyMsgs = make([]*msgmodel.MsgItemInMem, 0)
		}

		returnItems[idx] = &core.ConvListItem{
			ConvId:       mebConv.Id,
			ConvType:     mebConv.Type,
			Icon:         mebConv.Icon,
			Title:        mebConv.Title,
			RelationId:   mebConv.RelationId,
			Remark:       mebConv.Remark,
			PinTop:       mebConv.PinTop,
			NoDisturb:    mebConv.NoDisturb,
			MsgSeq:       conv.MsgSeq,
			LastMsg:      conv.LastMsg,
			BrowseMsgSeq: mebConv.BrowseMsgSeq,
			UnreadCount:  mebConv.UnreadCount,
			Cts:          mebConv.Cts,
			Uts:          mebConv.Uts,
			RecentlyMsgs: recentlyMsgs,
		}
	}

	return returnItems, true
}

func (cm *convManager) p2pConvHandle(convId string, msgId int64, convType chatconst.ConvType, pa core.MsgComingParam) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Second)
	defer cancel()

	lg := mylog.AppLogger()

	// 处理会话
	chrVal, convErr := cm.convSegLock.WithLockManual(
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

				return convHandleResult{
					conv:      c,
					convItems: conv.Items,
				}, nil
				// to do
			} else { // 记录不存在
				lg.Debug().Str("conv_id", convId).Msg("conversion was not exist in db, create and save now")
				msgQue := &deque.Deque[*msgmodel.MsgItemInMem]{}
				msgQue.Grow(cm.msgCap)
				c = &core.Conversation{
					Id:           convId,
					Type:         convType,
					Members:      []string{pa.Sender, pa.Receiver},
					RecentlyMsgs: msgQue,
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
			msgSeq    = chr.conv.MsgSeq
		)

		for _, convItem := range chr.convItems {
			browseSeq = convItem.BrowseMsgSeq

			unreadCnt := msgSeq - browseSeq
			if unreadCnt < 0 {
				unreadCnt = 0
			}

			uid := convItem.OwnerUid
			_, _ = cm.uidSegLock.WithLockManual(uid, func(lock *sync.RWMutex) (any, error) {
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

	seq, lastTs, err := cm.cr.ModifyConvTree(ctx, pa.Sender, convId, msgId)

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
	}, cm.doneChan, []any{pd.ConvId, pd.MsgSeq, pd.MsgId, pd.ConvLastActiveTs, pd.Sender, pd.Members})

	return err
}

func (cm *convManager) dbConv2local(conv *convrepo.Conv) *core.Conversation {
	msgQue := &deque.Deque[*msgmodel.MsgItemInMem]{}
	msgQue.Grow(cm.msgCap)
	c := &core.Conversation{
		Id:   conv.ConvId,
		Type: chatconst.ConvType(conv.Items[0].ConvType),
		Members: usli.Conv(conv.Items, func(item *convrepo.ConvItem) string {
			return item.OwnerUid
		}),
		RecentlyMsgs: msgQue,
		LastActiveTs: conv.LastActiveTs,
		LastMsgId:    conv.LastMsgId,
		MsgSeq:       conv.MsgSeq,
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

			msgId, ok := pt.Args[2].(int64)
			if !ok {
				continue
			}

			lastTs, ok := pt.Args[3].(int64)
			if !ok {
				continue
			}

			sendUid, ok := pt.Args[4].(string)
			if !ok {
				continue
			}

			members, ok := pt.Args[5].([]string)
			if !ok {
				continue
			}

			lg := mylog.AppLogger()
			if pt.Error != nil {
				lg.Error().Stack().Str("conv_id", convId).Int64("msg_seq", msgSeq).Err(pt.Error).Msg("mq msg async send failed")
				continue
			}

			_, _ = cm.convSegLock.WithLockManual(
				convId,
				func(lock *sync.RWMutex) (any, error) {
					lock.Lock()

					if conv, ok := cm.convId2items[convId]; ok {
						lastMsgId := conv.LastMsgId
						if msgId > lastMsgId {
							conv.LastMsgId = msgId
						}

						convMsgSeq := conv.MsgSeq
						if msgSeq > convMsgSeq {
							conv.MsgSeq = msgSeq
						}

						lastActiveTs := conv.LastActiveTs
						if lastTs > lastActiveTs {
							conv.LastActiveTs = lastTs
						}
					}

					lock.Unlock()
					return nil, nil
				})

			// members是不包括发送者的
			members = append(members, sendUid)

			for _, uid := range members {
				isSelf := uid == sendUid
				_, _ = cm.uidSegLock.WithLockManual(
					uid,
					func(lock *sync.RWMutex) (any, error) {
						lock.Lock()
						if convWrap, ok := cm.uid2convItems[uid]; ok {
							if convItem, ok := convWrap.ConvId2Item[convId]; ok {
								convItem.Uts = lastTs
								bmq := convItem.BrowseMsgSeq
								if isSelf {
									// 自身未读数清零
									if msgSeq > bmq {
										convItem.BrowseMsgSeq = msgSeq
									}

									convItem.UnreadCount = 0
								} else {
									unReadCnt := msgSeq - bmq
									if unReadCnt < 0 {
										unReadCnt = 0
									}

									convItem.UnreadCount = unReadCnt
								}
							}
						}

						lock.Unlock()
						return nil, nil
					})
			}

			lg.Trace().Str("conv_id", convId).Int64("msg_seq", msgSeq).Msg("mq msg async send success")
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
