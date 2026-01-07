package core

import (
	"github.com/sweemingdow/gmicro_pkg/pkg/graceful"
	"github.com/sweemingdow/sdim/external/eglobal/chatconst"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst/payload/msgpd"
	"github.com/sweemingdow/sdim/external/emodel/msgmodel"
	"github.com/sweemingdow/sdim/external/erpc/rpctopic"
)

type MsgComingParam struct {
	Sender         string
	Receiver       string
	ChatType       chatconst.ChatType
	ReqId          string
	Ttl            int32
	ClientUniqueId string
	MsgContent     *msgmodel.MsgContent
	IsConvType     bool
}

type MsgComingResult struct {
	MsgId  int64
	ConvId string
	Err    error
}

func MsgComingParamFrom(req rpctopic.MsgComingReq, reqId string) MsgComingParam {
	return MsgComingParam{
		Sender:         req.Sender,
		Receiver:       req.Receiver,
		ChatType:       req.ChatType,
		ReqId:          reqId,
		Ttl:            req.Ttl,
		ClientUniqueId: req.ClientUniqueId,
		MsgContent:     req.MsgContent,
	}
}

func MsgComingResultTo(mcr MsgComingResult, clientUniqueId string) rpctopic.MsgComingResp {
	return rpctopic.MsgComingResp{
		MsgId:          mcr.MsgId,
		ClientUniqueId: clientUniqueId,
		ConvId:         mcr.ConvId,
	}
}

// 会话管理器
type ConvManager interface {
	graceful.Gracefully

	OnMsgComing(pa MsgComingParam) MsgComingResult

	OnMsgStored(pd *msgpd.MsgSendReceivePayload)
}
