package model

type TelegramUser struct {
	Id        int64  `json:"id,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
	PhotoUrl  string `json:"photo_url,omitempty"`
	AuthDate  int64  `json:"auth_date,omitempty"`
	Hash      string `json:"hash,omitempty"`
}
