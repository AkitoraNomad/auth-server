//go:build integration

package http

import (
	"auth-server/internal"
	"auth-server/internal/model"
	authoauth "auth-server/internal/oauth2"
	"auth-server/internal/password"
	"auth-server/internal/render"
	"auth-server/internal/store"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	oauth "github.com/go-oauth2/oauth2/v4"
	"github.com/go-oauth2/oauth2/v4/server"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
)

// Each test owns a new PostgreSQL database and a random Redis key prefix.
// The hook changes only key arguments; Lua executes on the real Redis server.
type namespaceHook struct{ prefix string }

func (h namespaceHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h namespaceHook) prefixKeys(cmd redis.Cmder) error {
	args := cmd.Args()
	start, end := 1, 2
	switch strings.ToLower(cmd.Name()) {
	case "get", "set", "setnx", "setex", "psetex", "expire", "pexpire", "ttl", "pttl", "incr":
	case "del", "exists":
		end = len(args)
	case "eval", "evalsha":
		count, err := strconv.Atoi(fmt.Sprint(args[2]))
		if err != nil {
			return err
		}
		start, end = 3, 3+count
	case "hello", "client", "auth", "ping", "multi", "exec":
		return nil
	default:
		return fmt.Errorf("unhandled integration Redis command %q", cmd.Name())
	}
	for i := start; i < end; i++ {
		key := fmt.Sprint(args[i])
		if !strings.HasPrefix(key, h.prefix) {
			args[i] = h.prefix + key
		}
	}
	return nil
}
func (h namespaceHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if err := h.prefixKeys(cmd); err != nil {
			return err
		}
		return next(ctx, cmd)
	}
}
func (h namespaceHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			if err := h.prefixKeys(cmd); err != nil {
				return err
			}
		}
		return next(ctx, cmds)
	}
}

type integrationFixture struct {
	srv      *server.Server
	engine   http.Handler
	userID   int64
	password string
	prefix   string
	redisRaw *redis.Client
}

func integrationServer(t *testing.T) *integrationFixture {
	t.Helper()
	t.Chdir("../..")
	dsn, addr := os.Getenv("AUTH_TEST_POSTGRES"), os.Getenv("AUTH_TEST_REDIS")
	if dsn == "" || addr == "" {
		t.Skip("set AUTH_TEST_POSTGRES and AUTH_TEST_REDIS for isolated live-service regression")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	control, err := sqlx.ConnectContext(ctx, "pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := "auth_regression_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := control.ExecContext(ctx, `CREATE DATABASE "`+name+`"`); err != nil {
		control.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if internal.DB != nil {
			internal.DB.Close()
		}
		if _, err := control.Exec(`DROP DATABASE "` + name + `" WITH (FORCE)`); err != nil {
			t.Errorf("cleanup database: %v", err)
		}
		control.Close()
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	query := u.Query()
	query.Set("search_path", "auth")
	u.RawQuery = query.Encode()
	internal.DB, err = sqlx.ConnectContext(ctx, "pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile("db/postgres.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := internal.DB.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	prefix := name + ":"
	raw := redis.NewClient(&redis.Options{Addr: addr, DialTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second})
	client := redis.NewClient(raw.Options())
	client.AddHook(namespaceHook{prefix})
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	internal.Rdb = client
	t.Cleanup(func() {
		client.Close()
		// Never FLUSHDB: remove only this test's randomly named keys.
		iter := raw.Scan(context.Background(), 0, prefix+"*", 100).Iterator()
		for iter.Next(context.Background()) {
			if err := raw.Del(context.Background(), iter.Val()).Err(); err != nil {
				t.Errorf("cleanup Redis: %v", err)
			}
		}
		if err := iter.Err(); err != nil {
			t.Error(err)
		}
		raw.Close()
	})
	cfg := internal.Config{Host: "127.0.0.1", Port: 8080, Issuer: "http://localhost"}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cfg.JWT = []internal.JWTConfig{{Kid: "integration", Alg: "RS256", Sec: string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))}}
	cfg.Cloudflare.Turnstile.Key, cfg.Cloudflare.Turnstile.Secret = "fixture", "fixture"
	cfg.Admin.ClientID, cfg.Admin.DisplayName = 2, "Regression admin"
	cfg.Admin.RedirectURIs = []string{"http://localhost/admin-page/callback"}
	cfg.Admin.Scopes, cfg.Admin.GrantTypes = []string{"manage:system", "manage:clients"}, []string{"authorization_code", "refresh_token"}
	internal.AuthServerConfig = cfg
	if err := authoauth.LoadJWTConfig(); err != nil {
		t.Fatal(err)
	}
	if err := render.LoadTemplates("template/*"); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureAdminClient(); err != nil {
		t.Fatal(err)
	}
	hash, err := password.Hash("Regression-password-123!")
	if err != nil {
		t.Fatal(err)
	}
	user := &model.User{Email: "regression@example.invalid", DisplayName: "Regression admin"}
	pt := model.ProviderEmailPassword
	provider, _ := json.Marshal(map[string]string{"password": hash})
	if err := store.UserRepo.AddUser(user, model.AuthUserProvider{LoginKey: user.Email, ProviderType: &pt, ProviderData: provider}); err != nil {
		t.Fatal(err)
	}
	if _, err := internal.DB.Exec("INSERT INTO auth_admin(user_id) VALUES($1)", user.ID); err != nil {
		t.Fatal(err)
	}
	srv := authoauth.InitServer()
	return &integrationFixture{srv: srv, engine: CreateGinEngine(srv, os.DirFS("auth-server-webui/dist")), userID: user.ID, password: "Regression-password-123!", prefix: prefix, redisRaw: raw}
}
func (f *integrationFixture) token(t *testing.T, scopes string) oauth.TokenInfo {
	t.Helper()
	verifier := strings.Repeat("v", 43)
	digest := sha256.Sum256([]byte(verifier))
	code, err := f.srv.Manager.GenerateAuthToken(context.Background(), oauth.Code, &oauth.TokenGenerateRequest{ClientID: "2", UserID: strconv.FormatInt(f.userID, 10), RedirectURI: internal.AuthServerConfig.Admin.RedirectURIs[0], Scope: scopes, CodeChallenge: base64.RawURLEncoding.EncodeToString(digest[:]), CodeChallengeMethod: oauth.CodeChallengeS256})
	if err != nil {
		t.Fatal(err)
	}
	token, err := f.srv.Manager.GenerateAccessToken(context.Background(), oauth.AuthorizationCode, &oauth.TokenGenerateRequest{ClientID: "2", Code: code.GetCode(), RedirectURI: internal.AuthServerConfig.Admin.RedirectURIs[0], CodeVerifier: verifier})
	if err != nil {
		t.Fatal(err)
	}
	return token
}
func (f *integrationFixture) request(method, path, token string, body io.Reader) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	f.engine.ServeHTTP(w, req)
	return w
}

func TestLivePostgresRedisRegression(t *testing.T) {
	f := integrationServer(t)
	t.Run("ArraysAndConstraints", func(t *testing.T) {
		client := &model.AuthClient{DisplayName: "arrays", Public: true, Scopes: model.TextArray{"profile"}, RedirectURIs: model.TextArray{"https://app.example.invalid/cb?x=a,b&y=a%22b", "https://app.example.invalid/中文"}, GrantTypes: model.TextArray{"authorization_code"}}
		tt := model.TokenType(model.JWT)
		client.TokenType = &tt
		if err := store.ClientRepo.(*store.DbClientStore).Create(context.Background(), client); err != nil {
			t.Fatal(err)
		}
		actual, err := store.ClientRepo.GetByID(context.Background(), client.GetID())
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(actual.(*model.AuthClient).RedirectURIs) != fmt.Sprint(client.RedirectURIs) {
			t.Fatal("array changed on PostgreSQL roundtrip")
		}
		if _, err := internal.DB.Exec("INSERT INTO auth_user(email) VALUES('REGRESSION@example.invalid')"); err == nil {
			t.Fatal("case-insensitive email constraint missing")
		}
		if err := store.EnsureAdminClient(); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{"db/migrations/001_phase1_admin_spa.sql", "db/migrations/002_identity_constraints.sql"} {
			migration, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := internal.DB.Exec(string(migration)); err != nil {
				t.Fatalf("migration %s: %v", path, err)
			}
		}
	})
	token := f.token(t, "manage:system manage:clients")
	t.Run("AdminScopeAndRecentAuthentication", func(t *testing.T) {
		if w := f.request("GET", "/api/admin/me", token.GetAccess(), nil); w.Code != 200 {
			t.Fatalf("me: %d %s", w.Code, w.Body.String())
		}
		if w := f.request("DELETE", "/api/admin/clients/2", token.GetAccess(), nil); w.Code != 400 {
			t.Fatalf("management client delete: %d", w.Code)
		}
		limited := f.token(t, "manage:clients")
		if w := f.request("GET", "/api/admin/users", limited.GetAccess(), nil); w.Code != 403 {
			t.Fatalf("missing scope: %d", w.Code)
		}
	})
	t.Run("RefreshRotationReplayAndTimestamp", func(t *testing.T) {
		original := f.token(t, "manage:system")
		next, err := f.srv.Manager.RefreshAccessToken(context.Background(), &oauth.TokenGenerateRequest{ClientID: "2", Refresh: original.GetRefresh()})
		if err != nil {
			t.Fatal(err)
		}
		if next.GetRefresh() == original.GetRefresh() {
			t.Fatal("refresh was not rotated")
		}
		if _, err := f.srv.Manager.LoadAccessToken(context.Background(), original.GetAccess()); err == nil {
			t.Fatal("old access token survived rotation")
		}
		if _, err := f.srv.Manager.RefreshAccessToken(context.Background(), &oauth.TokenGenerateRequest{ClientID: "2", Refresh: original.GetRefresh()}); err == nil {
			t.Fatal("refresh replay accepted")
		}
		if _, err := f.srv.Manager.LoadAccessToken(context.Background(), next.GetAccess()); err == nil {
			t.Fatal("successor survived replay revocation")
		}
		var noLogin bool
		if err := internal.DB.Get(&noLogin, "SELECT last_login_at IS NULL FROM auth_user WHERE id=$1", f.userID); err != nil || !noLogin {
			t.Fatalf("token generation changed login timestamp: %v", err)
		}
	})
	t.Run("ConcurrentRefreshIsSingleUse", func(t *testing.T) {
		current := f.token(t, "manage:system")
		start := make(chan struct{})
		var wg sync.WaitGroup
		results := make(chan error, 4)
		for range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, err := f.srv.Manager.RefreshAccessToken(context.Background(), &oauth.TokenGenerateRequest{ClientID: "2", Refresh: current.GetRefresh()})
				results <- err
			}()
		}
		close(start)
		wg.Wait()
		close(results)
		successes := 0
		for err := range results {
			if err == nil {
				successes++
			}
		}
		if successes != 1 {
			t.Fatalf("successful concurrent refreshes=%d, want 1", successes)
		}
	})
	t.Run("RealLuaTTLAndRevocation", func(t *testing.T) {
		ctx := context.Background()
		current := f.token(t, "manage:system")
		family, err := internal.Rdb.Get(ctx, "oauth2:access-family:"+current.GetAccess()).Result()
		if err != nil {
			t.Fatal(err)
		}
		ttl, err := internal.Rdb.TTL(ctx, "oauth2:family:"+family).Result()
		if err != nil || ttl <= 0 {
			t.Fatalf("missing family TTL: %v %v", ttl, err)
		}
		if err := authoauth.RevokeUserSessions(ctx, f.userID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.srv.Manager.LoadAccessToken(ctx, current.GetAccess()); err == nil {
			t.Fatal("revoked token accepted")
		}
		fresh := f.token(t, "manage:system")
		if _, err := f.srv.Manager.LoadAccessToken(ctx, fresh.GetAccess()); err != nil {
			t.Fatal("fresh authorization failed after revocation", err)
		}
		w := f.request("POST", "/logout", fresh.GetAccess(), nil)
		if w.Code != 204 {
			t.Fatalf("logout: %d %s", w.Code, w.Body.String())
		}
		if _, err := f.srv.Manager.LoadRefreshToken(ctx, fresh.GetRefresh()); err == nil {
			t.Fatal("logout retained refresh")
		}
	})
	t.Run("Browser", func(t *testing.T) { runBrowserRegression(t, f) })
	t.Run("ConcurrentLastAdministratorProtection", func(t *testing.T) {
		var other int64
		if err := internal.DB.Get(&other, "INSERT INTO auth_user(email) VALUES('second@example.invalid') RETURNING id"); err != nil {
			t.Fatal(err)
		}
		if _, err := internal.DB.Exec("INSERT INTO auth_admin(user_id) VALUES($1)", other); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for _, id := range []int64{f.userID, other} {
			wg.Add(1)
			go func(id int64) { defer wg.Done(); results <- store.DeleteAdmin(context.Background(), id) }(id)
		}
		wg.Wait()
		close(results)
		failures := 0
		for err := range results {
			if errors.Is(err, store.ErrLastAdmin) {
				failures++
			} else if err != nil {
				t.Fatal(err)
			}
		}
		var count int
		if err := internal.DB.Get(&count, "SELECT count(*) FROM auth_admin"); err != nil || count != 1 || failures != 1 {
			t.Fatalf("last admin: count=%d failures=%d err=%v", count, failures, err)
		}
	})
}

type integrationTransport struct{}

func (integrationTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"success":true}`))}, nil
}

func runBrowserRegression(t *testing.T, f *integrationFixture) {
	python := os.Getenv("AUTH_TEST_BROWSER_PYTHON")
	if python == "" {
		t.Skip("set AUTH_TEST_BROWSER_PYTHON to a Python with Playwright")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://" + listener.Addr().String()
	internal.AuthServerConfig.Issuer = origin
	internal.AuthServerConfig.Admin.RedirectURIs = []string{origin + "/admin-page/callback"}
	if err := store.EnsureAdminClient(); err != nil {
		t.Fatal(err)
	}
	transport := turnstileClient.GetClient().Transport
	turnstileClient.SetTransport(integrationTransport{})
	t.Cleanup(func() { turnstileClient.SetTransport(transport) })
	web := httptest.NewUnstartedServer(f.engine)
	web.Listener.Close()
	web.Listener = listener
	web.Start()
	defer web.Close()
	cmd := exec.Command(python, "test/browser/regression.py")
	cmd.Env = append(os.Environ(), "AUTH_TEST_ORIGIN="+origin, "AUTH_TEST_EMAIL=regression@example.invalid", "AUTH_TEST_PASSWORD="+f.password)
	output, err := cmd.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		t.Fatal(err)
	}
	var logged bool
	if err := internal.DB.Get(&logged, "SELECT last_login_at IS NOT NULL FROM auth_user WHERE id=$1", f.userID); err != nil || !logged {
		t.Fatalf("credential login did not update timestamp: %v", err)
	}
}
