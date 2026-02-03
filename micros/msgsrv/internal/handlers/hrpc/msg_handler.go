package hrpc

import (
	"context"
	"github.com/lesismal/arpc"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/parser/json"
	"github.com/sweemingdow/gmicro_pkg/pkg/server/srpc"
	"github.com/sweemingdow/gmicro_pkg/pkg/server/srpc/rpccall"
	"github.com/sweemingdow/sdim/external/eglobal/chatconst"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst/payload/msgpd"
	"github.com/sweemingdow/sdim/external/emodel/msgmodel"
	"github.com/sweemingdow/sdim/external/erpc/rpcmsg"
	"github.com/sweemingdow/sdim/micros/msgsrv/internal/repostories/msgrepo"
	"golang.org/x/sync/errgroup"
	"math"
	"sync"
	"time"
)

type MsgHandler struct {
	msgResp msgrepo.MsgRepository
	dl      *mylog.DecoLogger
}

func NewMsgHandler(msgResp msgrepo.MsgRepository) *MsgHandler {
	return &MsgHandler{
		msgResp: msgResp,
		dl:      mylog.NewDecoLogger("msgHandlerLogger"),
	}
}

func (mh *MsgHandler) HandlerBatchConvRecentlyMsgs(c *arpc.Context) {
	var req rpcmsg.BatchConvRecentlyMsgsReq
	if ok := srpc.BindAndWriteLoggedIfError(c, &req); !ok {
		return
	}

	mh.dl.Trace().Strs("conv_ids", req.ConvIds).Msg("receive batch conv recently msgs")

	if len(req.ConvIds) == 0 {
		srpc.WriteLoggedIfError(c, rpcmsg.BatchConvRecentlyMsgsResp{
			ConvId2Msgs: make(map[string][]*msgmodel.MsgItemInMem),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var (
		eg          errgroup.Group
		convId2msgs = make(map[string][]*msgmodel.MsgItemInMem, len(req.ConvIds))
		mu          sync.Mutex
	)

	var concurrency = len(req.ConvIds) / 2
	if concurrency <= 0 {
		concurrency = 1
	}

	eg.SetLimit(concurrency)

	for _, convId := range req.ConvIds {
		convId := convId
		eg.Go(func() error {
			msgs, ie := mh.msgResp.FindConvMsgs(ctx, convId, math.MaxInt64, req.Limit)
			if ie != nil {
				return ie
			}

			items := make([]*msgmodel.MsgItemInMem, len(msgs))
			for idx, msg := range msgs {

				var smb msgpd.Msg
				ie = json.Parse([]byte(msg.MsgBody), &smb)
				if ie != nil {
					continue
				}

				items[idx] = &msgmodel.MsgItemInMem{
					MsgId:      msg.Id,
					ConvId:     convId,
					Sender:     msg.Sender,
					Receiver:   msg.Receiver,
					ChatType:   chatconst.ChatType(msg.ChatType),
					MsgType:    msgmodel.MsgType(msg.MsgType),
					Content:    smb.Content,
					SenderInfo: smb.SenderInfo,
					MegSeq:     msg.Seq,
					Cts:        msg.Cts,
				}
			}

			mu.Lock()
			convId2msgs[convId] = items
			mu.Unlock()

			return nil
		})
	}

	err := eg.Wait()
	if err != nil {
		srpc.WriteLoggedIfError(c, rpccall.SimpleErrDesc(err.Error()))
		return
	}

	srpc.WriteLoggedIfError(
		c,
		rpcmsg.BatchConvRecentlyMsgsResp{
			ConvId2Msgs: convId2msgs,
		},
	)
}
