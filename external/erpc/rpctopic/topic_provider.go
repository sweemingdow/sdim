package rpctopic

import (
	"github.com/sweemingdow/gmicro_pkg/pkg/server/srpc/rpccall"
	"github.com/sweemingdow/sdim/external/eglobal/chatconst"
	"github.com/sweemingdow/sdim/external/emodel/msgmodel"
)

type MsgComingReq struct {
	ConvId         string               `json:"convId,omitempty"`
	Sender         string               `json:"sender,omitempty"`
	Receiver       string               `json:"receiver,omitempty"`
	ChatType       chatconst.ChatType   `json:"chatType,omitempty"`
	Ttl            int32                `json:"ttl,omitempty"`
	ClientUniqueId string               `json:"clientUniqueId,omitempty"` // 客户端唯一id
	MsgContent     *msgmodel.MsgContent `json:"msgContent,omitempty"`
}

type MsgComingResp struct {
	MsgId          int64          `json:"msgId,omitempty"`
	ClientUniqueId string         `json:"clientUniqueId,omitempty"` // 客户端唯一id
	ConvId         string         `json:"convId,omitempty"`
	MsgSeq         int64          `json:"msgSeq,omitempty"` // 消息序列号
	SendTs         int64          `json:"sendTs,omitempty"` // 服务器时间戳
	Extra          map[string]any `json:"extra"`
}

// @rpc_server topic_server
//
//go:generate rpcgen -type TopicRpcProvider
type TopicRpcProvider interface {
	// @path /msg_coming
	// @timeout 400ms
	MsgComing(req rpccall.RpcReqWrapper[MsgComingReq]) (rpccall.RpcRespWrapper[MsgComingResp], error)
}
