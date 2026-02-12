package erespcode

const (
	GroupFrozen    = "200000" // 群被冻结
	GroupDismissed = "200001" // 群被解散
	GroupForbidden = "200002" // 群被禁言
)

const (
	GroupMebNotIn     = "200100" // 成员不在群内
	GroupMebBeKicked  = "200101" // 群成员被移出群聊
	GroupMebForbidden = "200102" // 群成员被禁言
)

var topicRespCode2text = map[string]string{
	GroupMebNotIn:     "meb not in group",
	GroupFrozen:       "group frozen",
	GroupDismissed:    "group dismissed",
	GroupForbidden:    "group forbidden",
	GroupMebBeKicked:  "group meb be kicked",
	GroupMebForbidden: "group meb forbidden",
}
