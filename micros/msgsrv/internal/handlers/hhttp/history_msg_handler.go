package hhttp

import (
	"context"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/sweemingdow/gmicro_pkg/pkg/parser/json"
	"github.com/sweemingdow/sdim/external/eglobal/chatconst"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst/payload/msgpd"
	"github.com/sweemingdow/sdim/external/emodel/msgmodel"
	"github.com/sweemingdow/sdim/micros/msgsrv/internal/repostories/msgrepo"
	"github.com/sweemingdow/sdim/pkg/wrapper"
	"strconv"
	"time"
)

const (
	HistoryMsgsOneCount uint8 = 50
)

type HistoryMsgHandler struct {
	mr msgrepo.MsgRepository
}

func NewHistoryMsgHandler(mr msgrepo.MsgRepository) *HistoryMsgHandler {
	return &HistoryMsgHandler{
		mr: mr,
	}
}

func (hmh *HistoryMsgHandler) HandleConvHistoryMsgs(c *fiber.Ctx) error {
	convId := c.Query("conv_id")
	if convId == "" {
		return fmt.Errorf("conv_id is required")
	}

	lastMsgIdVal := c.Query("last_msg_id")

	lastMsgId, err := strconv.ParseInt(lastMsgIdVal, 10, 64)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	msgs, err := hmh.mr.FindConvMsgs(ctx, convId, lastMsgId, HistoryMsgsOneCount)
	if err != nil {
		return err
	}

	var msgItems []*msgmodel.MsgItemInMem
	if len(msgs) == 0 {
		msgItems = make([]*msgmodel.MsgItemInMem, 0)
	} else {
		msgItems = make([]*msgmodel.MsgItemInMem, len(msgs))
		for idx, msg := range msgs {
			var smb msgpd.Msg
			err = json.Parse([]byte(msg.MsgBody), &smb)
			if err != nil {
				continue
			}

			msgItems[idx] = &msgmodel.MsgItemInMem{
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

	}

	if contents, err := json.Fmt(wrapper.RespOk(msgItems)); err == nil {
		return c.Send(contents)
	} else {
		return err
	}
}
