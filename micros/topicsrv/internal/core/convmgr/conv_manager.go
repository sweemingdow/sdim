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
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst/payload/convpd"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst/payload/msgpd"
	"github.com/sweemingdow/sdim/external/emodel/msgmodel"
	"github.com/sweemingdow/sdim/external/erpc/rpcmsg"
	"github.com/sweemingdow/sdim/external/erpc/rpcuser"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/core"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/repostories/convrepo"
	"golang.org/x/sync/singleflight"
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

	// 操作会话的分段锁
	convSegLock *guc.SegmentRwLock[string]
	// 操作用户会话的分段锁
	uidSegLock   *guc.SegmentRwLock[string]
	done         chan struct{}
	closed       atomic.Bool
	cr           convrepo.ConvRepository
	nsdPd        *cnsq.NsqProducer
	doneChan     chan *nsq.ProducerTransaction
	userProvider rpcuser.UserInfoRpcProvider
	msgProvider  rpcmsg.MsgProvider
	msgCap       int
	sfg          singleflight.Group
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
	convAdd   bool
}

func (cm *convManager) OnMsgComing(pa core.MsgComingParam) core.MsgComingResult {
	ve := msgmodel.ValidateMsgContent(pa.MsgContent)

	if ve != nil {
		return core.MsgComingResult{
			Err: ve,
		}
	}

	convId := generateP2pConvId(pa)
	var mcr = core.MsgComingResult{
		MsgId:  sfid.Next(),
		ConvId: convId,
	}

	convType := chatconst.GetConvTypeWithChatType(pa.ChatType)
	if pa.IsConvType {
		if convType == chatconst.P2pConv { // 单聊
			msgSeq, err := cm.p2pConvHandle(convId, mcr.MsgId, convType, pa)
			if err != nil {
				mcr.Err = err
				return mcr
			}

			mcr.MsgSeq = msgSeq
		} else if convType == chatconst.GroupConv { // 群聊
			msgSeq, err := cm.groupConvHandle(convId, mcr.MsgId, convType, pa)
			if err != nil {
				mcr.Err = err
				return mcr
			}

			mcr.MsgSeq = msgSeq
		}
	} else { // Danmaku

	}

	return mcr
}

func (cm *convManager) OnMsgStored(pd *msgpd.MsgForwardPayload) core.MsgStoredResult {
	// 更新会话的最新一条消息 & 会话的最近消息
	convId := pd.ConvId

	msr := core.MsgStoredResult{}

	_, _ = cm.convSegLock.WithLock(
		convId,
		func() (any, error) {
			var lastMsg *msgmodel.LastMsg
			if conv, ok := cm.convId2items[convId]; ok {
				if pd.MsgId >= conv.LastMsgId {
					conv.LastMsgId = pd.MsgId
					// 更新最后一条消息
					lastMsg = &msgmodel.LastMsg{
						MsgId:      pd.MsgId,
						SenderInfo: pd.Msg.SenderInfo,
						Content:    pd.Msg.Content,
					}

					conv.LastMsg = lastMsg

					msr.LastMsg = lastMsg
					msr.LastActiveTs = conv.LastActiveTs
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

	allMebs := append([]string{pd.Sender}, pd.Members...)
	uid2unreadCnt := make(map[string]int64, len(allMebs))
	for _, mebUid := range allMebs {
		cm.uidSegLock.PureRead(
			mebUid,
			func() {
				mebWrap, ok := cm.uid2convItems[mebUid]
				if !ok {
					return
				}

				convItem, ok := mebWrap.ConvId2Item[convId]
				if !ok {
					return
				}

				uid2unreadCnt[mebUid] = convItem.UnreadCount
			},
		)
	}

	msr.Uid2UnreadCount = uid2unreadCnt
	return msr
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
			msgSeq, _, lastActiveTs, userBrowseSeq = convrepo.TransferConvTreeForBatch(convTree)
			unreadCount = msgSeq - userBrowseSeq
			if unreadCount < 0 {
				unreadCount = 0
			}
		}

		var (
			recentlyMsgs = make([]*msgmodel.MsgItemInMem, 0)
			lastMsg      *msgmodel.LastMsg
		)
		msgs, ok := convId2msgs[item.ConvId]
		if ok {
			recentlyMsgs = msgs
			if len(recentlyMsgs) > 0 {
				val := recentlyMsgs[0]
				lastMsg = &msgmodel.LastMsg{
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

func (cm *convManager) ClearUnread(convId, uid string) error {
	lg := mylog.AppLogger().With().Str("uid", uid).Str("conv_id", convId).Logger()
	lg.Trace().Msgf("start clear unread")

	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()

	seq, err := cm.cr.ClearUnread(ctx, convId, uid)
	if err != nil {
		return err
	}

	lg = mylog.AppLogger()
	lg.Debug().Int64("seq", seq).Msg("clear unread count success")

	_, _ = cm.uidSegLock.WithLock(
		uid,
		func() (any, error) {
			mebWrap, ok := cm.uid2convItems[uid]
			if !ok {
				return nil, nil
			}

			conv, ok := mebWrap.ConvId2Item[convId]
			if !ok {
				return nil, nil
			}

			conv.BrowseMsgSeq = seq
			conv.UnreadCount = 0

			return nil, nil
		})

	return nil
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

func (cm *convManager) p2pConvHandle(convId string, msgId int64, convType chatconst.ConvType, pa core.MsgComingParam) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2000*time.Second)
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
				c = cm.dbConv2local(conv, pa.Sender, pa.Receiver)
				lg.Trace().Str("conv_id", convId).Msg("conversion was exist in db")

				cm.convId2items[convId] = c

				return convHandleResult{
					conv:      c,
					convItems: conv.Items,
				}, nil
			} else { // 记录不存在
				lg.Debug().Str("conv_id", convId).Msg("conversion was not exist in db, create and save now")

				msgQue := &deque.Deque[*msgmodel.MsgItemInMem]{}
				msgQue.Grow(cm.msgCap)
				c = &core.Conversation{
					Id:   convId,
					Type: convType,
					Members: map[string]struct{}{
						pa.Sender:   {},
						pa.Receiver: {},
					},
					RecentlyMsgs: msgQue,
				}

				req := rpccall.CreateIdReq(pa.ReqId, rpcuser.UsersUnitInfoReq{Uids: []string{pa.Sender, pa.Receiver}})
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
					convAdd:   true,
				}, nil
			}
		})

	if convErr != nil {
		return 0, convErr
	}

	chr := chrVal.(convHandleResult)

	// 处理用户会话
	// 库中存在会话, 就库中的会话同步到内存
	if len(chr.convItems) > 0 {
		convTreeVal, err, _ := cm.sfg.Do(
			convId,
			func() (interface{}, error) {
				return cm.cr.GetConvTreeWithP2p(ctx, convId)
			})

		if err != nil {
			return 0, err
		}

		convTree := convTreeVal.(map[string]string)

		msgSeq, _, lastActiveTs, uid2browseSeq := convrepo.TransferConvTreeForOrigin(
			convTree,
			usli.Conv(chr.convItems, func(item *convrepo.ConvItem) string {
				return item.OwnerUid
			}),
		)

		_, _ = cm.convSegLock.WithLock(
			convId,
			func() (any, error) {
				if convItem, ok := cm.convId2items[convId]; ok {
					if convItem.MsgSeq <= msgSeq {
						convItem.MsgSeq = msgSeq
						convItem.LastActiveTs = lastActiveTs
					}
				}
				return nil, nil
			})

		var browseSeq int64
		for _, convItem := range chr.convItems {
			browseSeq = uid2browseSeq[convItem.OwnerUid]

			unreadCnt := msgSeq - browseSeq
			if unreadCnt < 0 {
				unreadCnt = 0
			}

			uid := convItem.OwnerUid
			_, _ = cm.uidSegLock.WithLockManual(
				uid,
				func(lock *sync.RWMutex) (any, error) {
					lock.RLock()
					convWrap, ok := cm.uid2convItems[uid]
					lock.RUnlock()

					var updateConvFieldsFunc = func(mv *core.MemberConv, ci *convrepo.ConvItem) {
						mv.Title = ci.Title
						mv.Icon = ci.Icon
						mv.BrowseMsgSeq = browseSeq
						mv.UnreadCount = unreadCnt
					}

					if ok {
						lock.Lock()
						// 更新部分属性
						mebConv, ok := convWrap.ConvId2Item[convId]

						if ok {
							updateConvFieldsFunc(mebConv, convItem)

							lock.Unlock()
							return nil, nil
						}

						lock.Unlock()
					}

					lock.Lock()
					defer lock.Unlock()

					convWrap, ok = cm.uid2convItems[uid]

					if ok {
						if mebConv, ok := convWrap.ConvId2Item[convId]; ok {
							updateConvFieldsFunc(mebConv, convItem)
							return nil, nil
						}
					}

					convWrap = &core.MemberConvWrap{
						ConvId2Item: make(map[string]*core.MemberConv, 100),
						ConvItems:   make([]*core.MemberConv, 0, 100),
					}

					cm.uid2convItems[uid] = convWrap

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

					convWrap.ConvId2Item[convId] = mebConv
					convWrap.ConvItems = append(convWrap.ConvItems, mebConv)

					return nil, nil
				})
		}

		// 会话新增, mq到engine_server通知客户端
		if chr.convAdd {
			// mq到engine_server, 通知客户端会话新增
			pd := convpd.ConvAddEventPayload{
				ConvId:         convId,
				ConvType:       convType,
				ChatType:       pa.ChatType,
				Ts:             time.Now().UnixMilli(),
				Members:        make([]string, 0, len(chr.convItems)),
				MebId2UnitInfo: make(map[string]convpd.MemberUnitInfo, len(chr.convItems)),
				Sender:         pa.Sender,
				Receiver:       pa.Receiver,
				//RelationId:     pa.Receiver, // 单聊不需要
				FollowMsg: &msgmodel.LastMsg{
					MsgId:   msgId,
					Content: pa.MsgContent,
				},
			}

			for _, item := range chr.convItems {
				pd.Members = append(pd.Members, item.OwnerUid)
				pd.MebId2UnitInfo[item.OwnerUid] = convpd.MemberUnitInfo{
					Icon:  item.Icon,
					Title: item.Title,
				}
			}

			unitInfo, ok := pd.MebId2UnitInfo[pa.Sender]
			if ok {
				pd.FollowMsg.SenderInfo = msgmodel.SenderInfo{
					Nickname: unitInfo.Title,
					Avatar:   unitInfo.Icon,
				}
			}

			lg = lg.With().Str("conv_id", convId).Logger()

			lg.Debug().Msgf("conv add event will be published, pd=%+v", pd)

			payloads, err := json.Fmt(pd)
			if err == nil {
				err = cm.nsdPd.PublishAsync(
					cnsq.PublishParam{
						Topic:   nsqconst.ConvAddTopic,
						Payload: payloads,
					},
					nil,
					nil,
				)

				if err != nil {
					lg.Error().Stack().Err(err).Msgf("publish conv add event payload failed, pd=%+v", pd)
				}
			} else {
				lg.Error().Stack().Err(err).Msgf("format conv add event payload failed, pd=%+v", pd)
			}
		}
	}

	seq, lastTs, err := cm.cr.ModifyConvTree(ctx, pa.Sender, convId, msgId)

	if err != nil {
		return 0, err
	}

	// exclude self
	_, ok := chr.conv.Members[pa.Receiver]
	if !ok {
		return 0, errors.New("can not found receiver in conv:" + convId)
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
		Members:          []string{pa.Receiver},
		ReqId:            pa.ReqId,
		SendTs:           time.Now().UnixMilli(),
		MsgContent:       pa.MsgContent,
	})

	if err != nil {
		return 0, err
	}

	return seq, nil
}

func (cm *convManager) groupConvHandle(convId string, msgId int64, convType chatconst.ConvType, pa core.MsgComingParam) (int64, error) {
	return 0, nil
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

func (cm *convManager) dbConv2local(conv *convrepo.Conv, sender, receiver string) *core.Conversation {
	msgQue := &deque.Deque[*msgmodel.MsgItemInMem]{}
	msgQue.SetBaseCap(cm.msgCap)
	var mCap int
	if chatconst.ConvType(conv.ConvType) == chatconst.P2pConv {
		mCap = 2
	} else if chatconst.ConvType(conv.ConvType) == chatconst.CustomerConv {
		mCap = 3
	} else {
		mCap = 20
	}

	c := &core.Conversation{
		Id:           conv.ConvId,
		Type:         chatconst.ConvType(conv.ConvType),
		Members:      make(map[string]struct{}, mCap),
		RecentlyMsgs: msgQue,
		LastActiveTs: conv.LastActiveTs,
		LastMsgId:    conv.LastMsgId,
		MsgSeq:       conv.MsgSeq,
	}

	c.Members[sender] = struct{}{}
	c.Members[receiver] = struct{}{}

	return c
}

func (cm *convManager) localConv2db(c *core.Conversation, infoSli []*rpcuser.UnitInfoRespItem) *convrepo.Conv {
	conv := &convrepo.Conv{
		ConvId:   c.Id,
		Cts:      time.Now().UnixMilli(),
		ConvType: int8(c.Type),
		Items:    make([]*convrepo.ConvItem, 0, len(c.Members)),
	}

	for meb, _ := range c.Members {
		info, _ := usli.FindFirstIf(infoSli, func(val *rpcuser.UnitInfoRespItem) bool {
			// 找对方
			return val.Uid != meb
		})

		icon := info.Avatar
		title := info.Nickname

		conv.Items = append(conv.Items, &convrepo.ConvItem{
			OwnerUid:   meb,
			RelationId: info.Uid,
			Icon:       icon,
			Title:      title,
			Cts:        conv.Cts,
		})
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
				lg.Error().
					Stack().
					Str("conv_id", convId).
					Int64("msg_seq", msgSeq).
					Any("args", pt.Args).
					Err(pt.Error).
					Msg("mq msg async send failed")
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

func generateP2pConvId(pa core.MsgComingParam) string {
	if pa.Sender <= pa.Receiver {
		return fmt.Sprintf("p2p:%s:%s", pa.Sender, pa.Receiver)
	}

	return fmt.Sprintf("p2p:%s:%s", pa.Receiver, pa.Sender)
}

/*
func (cm *convManager) p2pConvHandle(convId string, msgId int64, convType chatconst.ConvType, pa core.MsgComingParam) (int64, error) {
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
					convAdd:   true,
				}, nil
			}
		})

	if convErr != nil {
		return 0, convErr
	}

	chr := chrVal.(convHandleResult)

	convAdd := chr.convAdd

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

		// mq到engine_server, 通知客户端会话新增
		if convAdd {
			pd := convpd.ConvAddEventPayload{
				ConvId:         convId,
				ConvType:       convType,
				ChatType:       pa.ChatType,
				Ts:             time.Now().UnixMilli(),
				Members:        make([]string, 0, len(chr.convItems)),
				MebId2UnitInfo: make(map[string]convpd.MemberUnitInfo, len(chr.convItems)),
				Sender:         pa.Sender,
				Receiver:       pa.Receiver,
				//RelationId:     pa.Receiver, // 单聊不需要
				FollowMsg: &msgmodel.LastMsg{
					MsgId:   msgId,
					Content: pa.MsgContent,
				},
			}

			for _, item := range chr.convItems {
				pd.Members = append(pd.Members, item.OwnerUid)
				pd.MebId2UnitInfo[item.OwnerUid] = convpd.MemberUnitInfo{
					Icon:  item.Icon,
					Title: item.Title,
				}
			}

			unitInfo, ok := pd.MebId2UnitInfo[pa.Sender]
			if ok {
				pd.FollowMsg.SenderInfo = msgmodel.SenderInfo{
					Nickname: unitInfo.Title,
					Avatar:   unitInfo.Icon,
				}
			}

			lg = lg.With().Str("conv_id", convId).Logger()

			lg.Debug().Msgf("conv add event will be published, pd=%+v", pd)

			payloads, err := json.Fmt(pd)
			if err == nil {
				err = cm.nsdPd.PublishAsync(
					cnsq.PublishParam{
						Topic:   nsqconst.ConvAddTopic,
						Payload: payloads,
					},
					nil,
					nil,
				)

				if err != nil {
					lg.Error().Stack().Err(err).Msgf("publish conv add event payload failed, pd=%+v", pd)
				}
			} else {
				lg.Error().Stack().Err(err).Msgf("format conv add event payload failed, pd=%+v", pd)
			}

		}
	}

	seq, lastTs, err := cm.cr.ModifyConvTree(ctx, pa.Sender, convId, msgId)

	if err != nil {
		return 0, err
	}

	// exclude self
	mebUid, ok := usli.FindFirstIf(chr.conv.Members, func(val string) bool {
		return val != pa.Sender
	})

	if !ok {
		return 0, errors.New("can not found receiver in conv:" + convId)
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
		return 0, err
	}

	return seq, nil
}
*/
