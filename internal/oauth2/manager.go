package oauth2

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"auth-server/internal"
	"auth-server/internal/model"
	storeImpl "auth-server/internal/store"

	oauth2 "github.com/go-oauth2/oauth2/v4"
	oauth2errors "github.com/go-oauth2/oauth2/v4/errors"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const tokenFamilyTTL = 7 * 24 * time.Hour
const (
	familyPrefix          = "oauth2:family:"
	familyRevokedPrefix   = "oauth2:family-revoked:"
	accessFamilyPrefix    = "oauth2:access-family:"
	refreshFamilyPrefix   = "oauth2:refresh-family:"
	refreshConsumedPrefix = "oauth2:refresh-consumed:"
	refreshLockPrefix     = "oauth2:refresh-lock:"
	userVersionPrefix     = "oauth2:user-version:"
	codePolicyPrefix      = "oauth2:code-policy:"
)

type grantPolicy struct {
	AuthTime    int64  `json:"auth_time"`
	UserID      string `json:"user"`
	ClientID    string `json:"client"`
	Version     string `json:"version"`
	Scope       string `json:"scope"`
	RedirectURI string `json:"redirect"`
}

type authenticatedVersionKey struct{}
type authenticatedTimeKey struct{}

type clientBoundManager struct {
	oauth2.Manager
	rdb *redis.Client
}

func newClientBoundManager(manager oauth2.Manager, rdb *redis.Client) *clientBoundManager {
	return &clientBoundManager{Manager: manager, rdb: rdb}
}

func (m *clientBoundManager) GenerateAuthToken(ctx context.Context, rt oauth2.ResponseType, req *oauth2.TokenGenerateRequest) (oauth2.TokenInfo, error) {
	if req == nil || rt != oauth2.Code || req.UserID == "" || !UserIsActive(ctx, req.UserID) {
		return nil, oauth2errors.ErrInvalidGrant
	}
	version, err := UserSessionVersion(ctx, req.UserID)
	if err != nil {
		return nil, err
	}
	authTime := time.Now()
	if req.Request != nil {
		authenticatedVersion, ok := req.Request.Context().Value(authenticatedVersionKey{}).(string)
		if !ok || authenticatedVersion != version {
			return nil, oauth2errors.ErrInvalidGrant
		}
		authTime, ok = req.Request.Context().Value(authenticatedTimeKey{}).(time.Time)
		if !ok || authTime.IsZero() {
			return nil, oauth2errors.ErrInvalidGrant
		}
	}
	ti, err := m.Manager.GenerateAuthToken(ctx, rt, req)
	if err != nil {
		return nil, err
	}
	policy := grantPolicy{AuthTime: authTime.Unix(), UserID: ti.GetUserID(), ClientID: ti.GetClientID(), Version: version, Scope: ti.GetScope(), RedirectURI: ti.GetRedirectURI()}
	data, err := json.Marshal(policy)
	if err != nil {
		return nil, err
	}
	if err = m.rdb.Set(ctx, codePolicyPrefix+ti.GetCode(), data, ti.GetCodeExpiresIn()).Err(); err != nil {
		return nil, err
	}
	return ti, nil
}

func (m *clientBoundManager) GenerateAccessToken(ctx context.Context, gt oauth2.GrantType, req *oauth2.TokenGenerateRequest) (oauth2.TokenInfo, error) {
	if req == nil {
		return nil, oauth2errors.ErrInvalidGrant
	}
	cli, err := m.authenticateClient(ctx, req)
	if err != nil {
		return nil, err
	}
	policy := grantPolicy{ClientID: req.ClientID, Scope: req.Scope, Version: "0"}
	switch gt {
	case oauth2.AuthorizationCode:
		if !validCodeVerifier(req.CodeVerifier) {
			return nil, oauth2errors.ErrInvalidCodeChallenge
		}
		data, err := m.rdb.Get(ctx, codePolicyPrefix+req.Code).Bytes()
		if err != nil || json.Unmarshal(data, &policy) != nil || policy.ClientID != req.ClientID || policy.RedirectURI != req.RedirectURI {
			return nil, oauth2errors.ErrInvalidAuthorizeCode
		}
		if !m.policyActive(ctx, policy) {
			return nil, oauth2errors.ErrInvalidGrant
		}
		if c, ok := cli.(*model.AuthClient); ok && !c.AllowsRedirectURI(req.RedirectURI) {
			return nil, oauth2errors.ErrInvalidRedirectURI
		}
	case oauth2.ClientCredentials:
		if cli.IsPublic() {
			return nil, oauth2errors.ErrInvalidClient
		}
	default:
		return nil, oauth2errors.ErrUnsupportedGrantType
	}
	if !scopeAllowed(cli, policy.Scope) {
		return nil, oauth2errors.ErrInvalidScope
	}
	ti, err := m.Manager.GenerateAccessToken(ctx, gt, req)
	if err != nil {
		return nil, err
	}
	if gt == oauth2.AuthorizationCode {
		_ = m.rdb.Del(ctx, codePolicyPrefix+req.Code).Err()
	}
	if ti.GetUserID() != policy.UserID || !m.policyActive(ctx, policy) {
		m.deleteTokenKeys(ctx, ti)
		return nil, oauth2errors.ErrInvalidGrant
	}
	if err = m.registerToken(ctx, ti, uuid.NewString(), policy); err != nil {
		m.deleteTokenKeys(ctx, ti)
		return nil, err
	}
	return ti, nil
}

func (m *clientBoundManager) RefreshAccessToken(ctx context.Context, req *oauth2.TokenGenerateRequest) (oauth2.TokenInfo, error) {
	if req == nil || req.Refresh == "" {
		return nil, oauth2errors.ErrInvalidRefreshToken
	}
	cli, err := m.authenticateClient(ctx, req)
	if err != nil {
		return nil, err
	}
	family, err := m.rdb.Get(ctx, refreshFamilyPrefix+req.Refresh).Result()
	if err != nil {
		return nil, oauth2errors.ErrInvalidRefreshToken
	}
	policy, err := m.familyPolicy(ctx, family)
	// Verify ownership before consuming or revoking anything. Another valid
	// client must not be able to revoke the victim's family using a stolen token.
	if err != nil || policy.ClientID != req.ClientID || !m.policyActive(ctx, policy) {
		return nil, oauth2errors.ErrInvalidRefreshToken
	}
	owner := uuid.NewString()
	locked, err := m.rdb.SetNX(ctx, refreshLockPrefix+req.Refresh, owner, 15*time.Second).Result()
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, oauth2errors.ErrInvalidRefreshToken
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		const release = `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`
		_, _ = m.rdb.Eval(cleanup, release, []string{refreshLockPrefix + req.Refresh}, owner).Result()
	}()
	consumed, err := m.rdb.Exists(ctx, refreshConsumedPrefix+req.Refresh).Result()
	if err != nil {
		return nil, err
	}
	if consumed != 0 {
		_ = m.revokeFamily(ctx, family)
		return nil, oauth2errors.ErrInvalidRefreshToken
	}
	old, err := m.LoadRefreshToken(ctx, req.Refresh)
	if err != nil {
		return nil, err
	}
	effective := old.GetScope()
	if strings.TrimSpace(req.Scope) != "" {
		effective = req.Scope
		for _, scope := range strings.Fields(effective) {
			if !model.HasScope(old.GetScope(), scope) {
				return nil, oauth2errors.ErrInvalidScope
			}
		}
	}
	if !scopeAllowed(cli, effective) {
		return nil, oauth2errors.ErrInvalidScope
	}
	claimed, err := m.rdb.SetNX(ctx, refreshConsumedPrefix+req.Refresh, family, tokenFamilyTTL).Result()
	if err != nil {
		return nil, err
	}
	if !claimed {
		_ = m.revokeFamily(ctx, family)
		return nil, oauth2errors.ErrInvalidRefreshToken
	}
	ti, err := m.Manager.RefreshAccessToken(ctx, req)
	if err != nil {
		_ = m.revokeFamily(ctx, family)
		return nil, err
	}
	if err = m.registerToken(ctx, ti, family, policy); err != nil {
		m.deleteTokenKeys(ctx, ti)
		_ = m.revokeFamily(ctx, family)
		return nil, err
	}
	return ti, nil
}

func (m *clientBoundManager) authenticateClient(ctx context.Context, req *oauth2.TokenGenerateRequest) (oauth2.ClientInfo, error) {
	cli, err := m.Manager.GetClient(ctx, req.ClientID)
	if err != nil || cli == nil {
		return nil, oauth2errors.ErrInvalidClient
	}
	if cli.IsPublic() {
		if req.ClientSecret != "" {
			return nil, oauth2errors.ErrInvalidClient
		}
		return cli, nil
	}
	if cli.GetSecret() == "" || subtle.ConstantTimeCompare([]byte(cli.GetSecret()), []byte(req.ClientSecret)) != 1 {
		return nil, oauth2errors.ErrInvalidClient
	}
	return cli, nil
}

func scopeAllowed(cli oauth2.ClientInfo, scope string) bool {
	if _, err := model.ParseScopes(scope); err != nil {
		return false
	}
	for _, name := range strings.Fields(scope) {
		if !cliHasScope(cli, name) {
			return false
		}
	}
	return true
}

// Publishing the token-family links, checking revocation and checking the user
// generation are one Redis operation. Tokens written by the library are not
// usable through this manager until this publication succeeds. A crash, expired
// lock or concurrent revocation therefore cannot expose an untracked successor.
const publishFamilyScript = `-- auth-publish-family-v1
if redis.call('exists', KEYS[1]) ~= 0 then return 0 end
if ARGV[1] ~= '' and (redis.call('get', KEYS[2]) or '0') ~= ARGV[2] then return 0 end
local prior = redis.call('get', KEYS[3])
if prior and prior ~= ARGV[3] then return 0 end
redis.call('set', KEYS[3], ARGV[3], 'EX', ARGV[4])
redis.call('set', KEYS[4], ARGV[5], 'EX', ARGV[4])
if ARGV[6] ~= '' then redis.call('set', KEYS[5], ARGV[5], 'EX', ARGV[4]) end
return 1`

func (m *clientBoundManager) registerToken(ctx context.Context, ti oauth2.TokenInfo, family string, policy grantPolicy) error {
	if !m.policyActive(ctx, policy) {
		return oauth2errors.ErrInvalidGrant
	}
	data, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	ttl := ti.GetAccessCreateAt().Add(ti.GetAccessExpiresIn()).Sub(time.Now())
	if ti.GetRefresh() != "" {
		ttl = ti.GetRefreshCreateAt().Add(ti.GetRefreshExpiresIn()).Sub(time.Now())
	}
	if ttl <= 0 || ttl > tokenFamilyTTL {
		return errors.New("token lifetime must be positive and at most seven days")
	}
	// Keep replay evidence longer than any valid token in this family.
	ok, err := m.rdb.Eval(ctx, publishFamilyScript, []string{
		familyRevokedPrefix + family, userVersionPrefix + policy.UserID, familyPrefix + family,
		accessFamilyPrefix + ti.GetAccess(), refreshFamilyPrefix + ti.GetRefresh(),
	}, policy.UserID, policy.Version, string(data), int64(tokenFamilyTTL/time.Second), family, ti.GetRefresh()).Int()
	if err != nil {
		return err
	}
	if ok != 1 {
		return oauth2errors.ErrInvalidGrant
	}
	return nil
}

func (m *clientBoundManager) familyPolicy(ctx context.Context, family string) (grantPolicy, error) {
	var policy grantPolicy
	revoked, err := m.rdb.Exists(ctx, familyRevokedPrefix+family).Result()
	if err != nil || revoked != 0 {
		return policy, oauth2errors.ErrInvalidGrant
	}
	data, err := m.rdb.Get(ctx, familyPrefix+family).Bytes()
	if err != nil {
		return policy, err
	}
	err = json.Unmarshal(data, &policy)
	return policy, err
}

func (m *clientBoundManager) policyActive(ctx context.Context, policy grantPolicy) bool {
	if policy.UserID == "" {
		return true
	}
	if !UserIsActive(ctx, policy.UserID) {
		return false
	}
	version, err := UserSessionVersion(ctx, policy.UserID)
	return err == nil && version == policy.Version
}

func (m *clientBoundManager) validateToken(ctx context.Context, ti oauth2.TokenInfo, familyKey string) error {
	family, err := m.rdb.Get(ctx, familyKey).Result()
	if err != nil {
		return oauth2errors.ErrInvalidGrant
	}
	policy, err := m.familyPolicy(ctx, family)
	if err != nil || policy.UserID != ti.GetUserID() || policy.ClientID != ti.GetClientID() || !m.policyActive(ctx, policy) {
		return oauth2errors.ErrInvalidGrant
	}
	return nil
}

func (m *clientBoundManager) LoadAccessToken(ctx context.Context, token string) (oauth2.TokenInfo, error) {
	ti, err := m.Manager.LoadAccessToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if err = m.validateToken(ctx, ti, accessFamilyPrefix+token); err != nil {
		return nil, err
	}
	return ti, nil
}

// Refreshing does not renew the authentication time. Sensitive administrator
// writes require a credential sign-in within the preceding fifteen minutes.
func (m *clientBoundManager) RecentlyAuthenticated(ctx context.Context, token string) bool {
	if _, err := m.LoadAccessToken(ctx, token); err != nil {
		return false
	}
	family, err := m.rdb.Get(ctx, accessFamilyPrefix+token).Result()
	if err != nil {
		return false
	}
	policy, err := m.familyPolicy(ctx, family)
	if err != nil || policy.AuthTime <= 0 || policy.UserID == "" {
		return false
	}
	age := time.Since(time.Unix(policy.AuthTime, 0))
	return age >= -time.Minute && age <= 15*time.Minute
}

func (m *clientBoundManager) LoadRefreshToken(ctx context.Context, token string) (oauth2.TokenInfo, error) {
	ti, err := m.Manager.LoadRefreshToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if err = m.validateToken(ctx, ti, refreshFamilyPrefix+token); err != nil {
		return nil, err
	}
	return ti, nil
}

func (m *clientBoundManager) revokeFamily(ctx context.Context, family string) error {
	return m.rdb.Set(ctx, familyRevokedPrefix+family, "1", tokenFamilyTTL).Err()
}

func (m *clientBoundManager) deleteTokenKeys(ctx context.Context, ti oauth2.TokenInfo) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	if ti.GetAccess() != "" {
		_ = m.Manager.RemoveAccessToken(cleanup, ti.GetAccess())
	}
	if ti.GetRefresh() != "" {
		_ = m.Manager.RemoveRefreshToken(cleanup, ti.GetRefresh())
	}
}

// UserSessionVersion is a persistent security generation; it must never expire
// and return to an earlier value while any session or credential may survive.
func UserSessionVersion(ctx context.Context, userID string) (string, error) {
	if internal.Rdb == nil {
		return "", errors.New("redis unavailable")
	}
	version, err := internal.Rdb.Get(ctx, userVersionPrefix+userID).Result()
	if errors.Is(err, redis.Nil) {
		// An evicted or lost generation must create a new random generation,
		// never reactivate old tokens by falling back to a fixed default.
		if err := internal.Rdb.SetNX(ctx, userVersionPrefix+userID, uuid.NewString(), 0).Err(); err != nil {
			return "", err
		}
		return internal.Rdb.Get(ctx, userVersionPrefix+userID).Result()
	}
	return version, err
}

func UserIsActive(ctx context.Context, userID string) bool {
	if userID == "" {
		return true
	}
	id, err := strconv.ParseInt(userID, 10, 64)
	if err != nil || id <= 0 {
		return false
	}
	user, err := storeImpl.UserRepo.GetUserByID(id)
	return err == nil && user != nil
}

// SessionIsActive rejects legacy sessions without a generation, stale sessions
// after forced logout, and sessions beyond the absolute twelve-hour lifetime.
func SessionIsActive(ctx context.Context, info *model.SessionUserInfo) bool {
	if info == nil || info.User == nil || info.AuthVersion == "" || info.LoginAt.IsZero() ||
		info.LoginAt.After(time.Now().Add(time.Minute)) || time.Since(info.LoginAt) > 12*time.Hour {
		return false
	}
	uid := strconv.FormatInt(info.User.ID, 10)
	version, err := UserSessionVersion(ctx, uid)
	return err == nil && version == info.AuthVersion && UserIsActive(ctx, uid)
}

// RevokeUserSessions invalidates existing sessions, codes and token families.
// New sign-ins bind to the new generation and are immediately usable.
func RevokeUserSessions(ctx context.Context, userID int64) error {
	if internal.Rdb == nil || userID <= 0 {
		return errors.New("invalid revocation request")
	}
	return internal.Rdb.Set(ctx, userVersionPrefix+strconv.FormatInt(userID, 10), uuid.NewString(), 0).Err()
}

func validCodeVerifier(v string) bool {
	if len(v) < 43 || len(v) > 128 {
		return false
	}
	for _, r := range v {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || strings.ContainsRune("-._~", r)) {
			return false
		}
	}
	return true
}
