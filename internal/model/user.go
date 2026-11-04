package model

import "time"

type AuthUser struct {
	IsDisabled      bool       `db:"is_disabled"`
	SecurityVersion int64      `db:"security_version"`
	ID              int64      `db:"id"`
	Email           string     `db:"email"`
	DisplayName     string     `db:"display_name"`
	LastLoginAt     *time.Time `db:"last_login_at"`
	BaseModel
}

// User represents a user in the system
type User struct {
	IsDisabled      bool       `json:"is_disabled"`
	SecurityVersion int64      `json:"-"`
	ID              int64      `json:"id"`
	Email           string     `json:"email"`
	DisplayName     string     `json:"display_name"`
	LastLoginAt     *time.Time `json:"last_login_at,omitempty"`
}

// SessionUserInfo 用于在会话中存储当前登录用户的上下文数据
type SessionUserInfo struct {
	UserSecurityVersion int64          `json:"user_security_version"`
	AuthVersion         string         `json:"auth_version"`
	User                *User          `json:"user"`
	ProviderType        *ProviderType  `json:"provider_type,omitempty"`
	LoginIP             string         `json:"login_ip,omitempty"`
	LoginClientID       string         `json:"login_client_id,omitempty"`
	LoginAt             time.Time      `json:"login_at"`
	Metadata            map[string]any `json:"metadata,omitempty"`
}

// NewUserFromAuth constructs a user from the auth user entity
func NewUserFromAuth(authUser AuthUser, lastLoginAt *time.Time) User {
	return User{
		IsDisabled:      authUser.IsDisabled,
		SecurityVersion: authUser.SecurityVersion,
		ID:              authUser.ID,
		Email:           authUser.Email,
		DisplayName:     authUser.DisplayName,
		LastLoginAt:     lastLoginAt,
	}
}

// NewSessionUserInfo constructs a SessionUserInfo to be stored in the session
func NewSessionUserInfo(user *User, providerType *ProviderType, loginIP, loginClientID string, loginAt time.Time, metadata map[string]any) *SessionUserInfo {
	return &SessionUserInfo{
		UserSecurityVersion: user.SecurityVersion,
		User:                user,
		ProviderType:        providerType,
		LoginIP:             loginIP,
		LoginClientID:       loginClientID,
		LoginAt:             loginAt,
		Metadata:            metadata,
	}
}
