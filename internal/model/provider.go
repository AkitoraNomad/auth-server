package model

import (
	"auth-server/internal/password"
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

type ProviderData interface {
	Validate(data any) bool
}

type AuthUserProvider struct {
	ID           int64           `db:"id"`
	UserID       int64           `db:"user_id"`
	LoginKey     string          `db:"login_key"`
	ProviderType *ProviderType   `db:"provider_type"`
	ProviderData json.RawMessage `db:"provider_data"`
	BaseModel
}

type ProviderType int

const (
	ProviderEmailPassword ProviderType = iota
	ProviderTelegram
)

func stringToProviderType(s string) (ProviderType, error) {
	switch s {
	case "EmailPassword":
		return ProviderEmailPassword, nil
	case "Telegram":
		return ProviderTelegram, nil
	default:
		return 0, fmt.Errorf("unknown ProviderType: %s", s)
	}
}

func (t *ProviderType) UnmarshalJSON(bytes []byte) error {
	var s string
	if err := json.Unmarshal(bytes, &s); err != nil {
		return err
	}
	providerType, err := stringToProviderType(s)
	if err != nil {
		return err
	}
	*t = providerType
	return nil
}

func (t *ProviderType) String() string {
	providerType := *t
	switch providerType {
	case ProviderEmailPassword:
		return "EmailPassword"
	case ProviderTelegram:
		return "Telegram"
	default:
		return "unknown"
	}
}

func (t *ProviderType) Value() (driver.Value, error) {
	s := t.String()
	if s == "unknown" {
		return nil, fmt.Errorf("unknown value %s", s)
	}
	return s, nil
}

func (t *ProviderType) Scan(value any) error {
	var strVal string
	switch v := value.(type) {
	case []uint8:
		strVal = string(v)
	case string:
		strVal = v
	default:
		return fmt.Errorf("ProviderType must be a string, got %T", value)
	}

	providerType, err := stringToProviderType(strVal)
	if err != nil {
		return err
	}
	*t = providerType
	return nil
}

type EmailPasswordProviderData struct {
	Password string `json:"password"`
}

func (p *EmailPasswordProviderData) Validate(data string) bool {
	return password.Verify(p.Password, data)
}
