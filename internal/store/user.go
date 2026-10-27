package store

import (
	"auth-server/internal"
	"auth-server/internal/model"
	"auth-server/internal/util"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/go-session/session/v3"
)

var UserRepo UserStore = &DbUserStore{}

type UserStore interface {
	GetUserByID(id int64) (*model.User, error)
	GetUserByCredentials(providerID string, providerType *model.ProviderType, data json.RawMessage, sessionStore session.Store) (user *model.User, err error)
	AddUser(user *model.User, provider model.AuthUserProvider) error
	UpdateLastLogin(userID int64) error
}

type DbUserStore struct {
}

func (m *DbUserStore) GetUserByID(id int64) (*model.User, error) {
	authUser := model.AuthUser{}
	if err := internal.DB.Get(&authUser, "select * from auth_user where id = $1", id); err != nil {
		return nil, err
	}
	user := model.NewUserFromAuth(authUser, authUser.LastLoginAt)
	return &user, nil
}

func (m *DbUserStore) GetUserByCredentials(
	loginKey string,
	providerType *model.ProviderType,
	data json.RawMessage,
	sessionStore session.Store,
) (*model.User, error) {
	if providerType == nil {
		return nil, errors.New("provider type required")
	}

	var verifiedTelegram *util.TelegramAuth
	if *providerType == model.ProviderTelegram {
		if internal.AuthServerConfig.Telegram.BotToken == "" {
			return nil, errors.New("telegram login is disabled")
		}
		// The selected local account must be derived from the signed Telegram
		// identity, never solely from a client-supplied login_key.
		tg, err := util.ParseTelegramCredential(
			data,
			util.DigestSHA256(internal.AuthServerConfig.Telegram.BotToken),
			time.Now(),
		)
		if err != nil {
			slog.Warn("invalid telegram credential", "err", err)
			return nil, errors.New("invalid_credentials")
		}
		verifiedKey := strconv.FormatInt(tg.ID, 10)
		if loginKey != verifiedKey {
			slog.Warn("telegram identity does not match login key")
			return nil, errors.New("invalid_credentials")
		}
		verifiedTelegram = tg
		loginKey = verifiedKey
		// A correctly signed widget response is still a bearer credential.
		// Consume it once across instances, for the entire acceptance window.
		if internal.Rdb == nil {
			return nil, errors.New("credential replay store unavailable")
		}
		canonical, err := json.Marshal(tg)
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256(canonical)
		accepted, err := internal.Rdb.SetNX(sessionStore.Context(), fmt.Sprintf("auth:telegram:%x", digest), "1", 6*time.Minute).Result()
		if err != nil || !accepted {
			return nil, errors.New("invalid_credentials")
		}
	}

	var providers []model.AuthUserProvider
	if err := internal.DB.Select(&providers, `select * from auth_user_provider where login_key = $1 and provider_type = $2`, loginKey, providerType); err != nil {
		return nil, err
	}
	if len(providers) == 0 {
		return nil, nil
	}
	if len(providers) != 1 {
		return nil, errors.New("ambiguous identity")
	}
	provider := providers[0]
	switch *provider.ProviderType {
	case model.ProviderEmailPassword:
		req := model.EmailPasswordProviderData{}
		if err := json.Unmarshal(data, &req); err != nil {
			return nil, err
		}
		repo := model.EmailPasswordProviderData{}
		if err := json.Unmarshal(provider.ProviderData, &repo); err != nil {
			return nil, err
		}
		if repo.Validate(req.Password) {
			return m.GetUserByID(provider.UserID)
		}
		return nil, errors.New("invalid_credentials")
	case model.ProviderTelegram:
		if verifiedTelegram == nil {
			return nil, errors.New("invalid_credentials")
		}
		tgUser := model.TelegramUser{
			Id:        verifiedTelegram.ID,
			FirstName: verifiedTelegram.FirstName,
			LastName:  verifiedTelegram.LastName,
			Username:  verifiedTelegram.Username,
			PhotoUrl:  verifiedTelegram.PhotoURL,
			AuthDate:  verifiedTelegram.AuthDate,
		}
		sessionStore.Set(internal.SessionKeyTelegramData, tgUser)
		if err := sessionStore.Save(); err != nil {
			return nil, err
		}
		return m.GetUserByID(provider.UserID)
	default:
		return nil, fmt.Errorf("unknown ProviderType: %v", providerType)
	}
}

func (m *DbUserStore) AddUser(user *model.User, provider model.AuthUserProvider) error {
	now := time.Now()
	e := model.AuthUser{
		Email:       user.Email,
		DisplayName: user.DisplayName,
		BaseModel: model.BaseModel{
			CreatedAt: &now,
			UpdatedAt: &now,
		},
	}

	// Start a transaction
	tx, err := internal.DB.Beginx()
	if err != nil {
		return err
	}

	defer tx.Rollback()

	var uCount = 0
	if err = tx.Get(&uCount, "SELECT COUNT(*) FROM auth_user WHERE lower(email) = lower($1)", user.Email); err != nil {
		return err
	}

	if uCount > 0 {
		return errors.New("user_exists")
	}

	insertUserSql := `INSERT INTO auth_user (email, display_name, created_at, updated_at) 
	              VALUES (:email, :display_name, :created_at, :updated_at) RETURNING id`
	rows, err := tx.NamedQuery(insertUserSql, e)
	if err != nil {
		return err
	}
	var userID int64
	if !rows.Next() {
		_ = rows.Close()
		return errors.New("create user returned no id")
	}
	if err = rows.Scan(&userID); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	user.ID = userID

	// Insert provider data
	provider.UserID = userID
	provider.BaseModel = model.BaseModel{
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	insertProviderSql := `INSERT INTO auth_user_provider (user_id, login_key, provider_type, provider_data, created_at, updated_at)
	              VALUES (:user_id, :login_key, :provider_type, :provider_data, :created_at, :updated_at)`

	_, err = tx.NamedExec(insertProviderSql, provider)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (m *DbUserStore) UpdateLastLogin(userID int64) error {
	now := time.Now()
	_, err := internal.DB.Exec("UPDATE auth_user SET last_login_at = $1 WHERE id = $2", now, userID)
	return err
}

func (m *DbUserStore) List() ([]model.User, error) {
	var authUsers []model.AuthUser
	if err := internal.DB.Select(&authUsers, "select * from auth_user order by id desc"); err != nil {
		return nil, err
	}
	users := make([]model.User, 0, len(authUsers))
	for _, au := range authUsers {
		users = append(users, model.NewUserFromAuth(au, au.LastLoginAt))
	}
	return users, nil
}

func (m *DbUserStore) Delete(id int64) error {
	tx, err := internal.DB.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := protectLastAdmin(tx, id); err != nil {
		return err
	}

	if _, err = tx.Exec("DELETE FROM auth_user_provider WHERE user_id = $1", id); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM auth_user WHERE id = $1", id); err != nil {
		return err
	}
	return tx.Commit()
}
