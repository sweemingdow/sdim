package erespcode

import "github.com/sweemingdow/gmicro_pkg/pkg/myerr"

func _newTopicRespErr(code string, data any) error {
	return myerr.NewRpcRespError(code, "", topicRespCode2text[code], data)
}

func NewMebNotInGroupErr() error {
	return _newTopicRespErr(GroupMebNotIn, nil)
}

func NewGroupFrozenErr(data any) error {
	return _newTopicRespErr(GroupFrozen, data)
}

func NewGroupDismissedErr() error {
	return _newTopicRespErr(GroupDismissed, nil)
}

func NewGroupForbiddenErr() error {
	return _newTopicRespErr(GroupForbidden, nil)
}

func NewMebBeKickedErr() error {
	return _newTopicRespErr(GroupMebBeKicked, nil)
}

func NewMebForbiddenErr(data any) error {
	return _newTopicRespErr(GroupMebForbidden, data)
}
