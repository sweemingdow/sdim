package core

import (
	"github.com/sweemingdow/gmicro_pkg/pkg/graceful"
	"github.com/sweemingdow/sdim/external/eglobal/chatconst"
	"github.com/sweemingdow/sdim/external/erpc/rpctopic"
)

type MsgComingParam struct {
	Sender     string
	Receiver   string
	ChatType   chatconst.ChatType
	ReqId      string
	Ttl        int32
	MsgBody    any
	IsConvType bool
}

type MsgComingResult struct {
	MsgId  int64
	ConvId string
	Err    error
}

func MsgComingParamFrom(req rpctopic.MsgComingReq, reqId string) MsgComingParam {
	return MsgComingParam{
		Sender:   req.Sender,
		Receiver: req.Receiver,
		ChatType: req.ChatType,
		ReqId:    reqId,
		Ttl:      req.Ttl,
		MsgBody:  req.MsgBody,
	}
}

func MsgComingResultTo(mcr MsgComingResult) rpctopic.MsgComingResp {
	return rpctopic.MsgComingResp{
		MsgId:  mcr.MsgId,
		ConvId: mcr.ConvId,
	}
}

// 会话管理器
type ConvManager interface {
	graceful.Gracefully

	OnMsgComing(pa MsgComingParam) MsgComingResult
}
