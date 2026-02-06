package notifypd

type NotifyPayload struct {
	NotifyType string         `json:"notifyType,omitempty"`
	SubType    string         `json:"subType,omitempty"`
	Members    []string       `json:"members,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
}
