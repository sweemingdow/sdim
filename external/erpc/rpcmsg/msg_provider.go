package rpcmsg

import "github.com/sweemingdow/sdim/external/emodel/msgmodel"

type BatchConvRecentlyMsgsReq struct {
	ConvIds []string `json:"convIds"`
	Limit   uint8    `json:"limit"`
}

type BatchConvRecentlyMsgsResp struct {
	ConvId2Msgs map[string][]*msgmodel.MsgItemInMem `json:"convId2Msgs"`
}

// @rpc_server msg_server
//
//go:generate rpcgen -type MsgProvider
type MsgProvider interface {
	// @path /batch_conv_recently_msgs
	// @timeout 3s
	BatchConvRecentlyMsgs(req BatchConvRecentlyMsgsReq) (BatchConvRecentlyMsgsResp, error)
}
