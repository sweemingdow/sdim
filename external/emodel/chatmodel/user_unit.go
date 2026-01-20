package chatmodel

type UidNickname struct {
	Uid      string `json:"uid,omitempty"`
	Nickname string `json:"nickname,omitempty"`
}

type UidNicknameAvatar struct {
	Uid      string `json:"uid,omitempty"`
	Nickname string `json:"nickname,omitempty"`
	Avatar   string `json:"avatar,omitempty"`
}
