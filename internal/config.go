package internal

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var AuthServerConfig Config

type JWTConfig struct {
	Kid string `yaml:"kid"`
	Alg string `yaml:"alg"`
	Sec string `yaml:"sec"`
}

type Config struct {
	Host  string `yaml:"host"`
	Port  int    `yaml:"port"`
	Redis struct {
		Host string `yaml:"host"`
		Port string `yaml:"port"`
	} `yaml:"redis"`
	DB           string      `yaml:"db"`
	DBSearchPath string      `yaml:"db-search-path"`
	Issuer       string      `yaml:"issuer"`
	JWT          []JWTConfig `yaml:"jwt"`
	Admin        struct {
		ClientID     int64    `yaml:"client-id"`
		DisplayName  string   `yaml:"display-name"`
		RedirectURIs []string `yaml:"redirect-uris"`
		Scopes       []string `yaml:"scopes"`
		GrantTypes   []string `yaml:"grant-types"`
	} `yaml:"admin"`
	Cloudflare struct {
		Turnstile struct {
			Key    string `yaml:"key"`
			Secret string `yaml:"secret"`
		} `yaml:"turnstile"`
	} `yaml:"cloudflare"`
	Telegram struct {
		BotName  string `yaml:"bot-name"`
		BotToken string `yaml:"bot-token"`
	} `yaml:"telegram"`
}

func init() {
	configPath, err := filepath.Abs("config.yaml")
	if err != nil {
		slog.Warn("failed to get absolute path of config file", "err", err)
		return
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		slog.Warn("failed to read config file or file not found", "path", configPath, "err", err)
		return
	}

	if err = yaml.Unmarshal(data, &AuthServerConfig); err != nil {
		slog.Error("failed to unmarshal config", "path", configPath, "err", err)
		return
	}

	slog.Info("successfully loaded config file", "path", configPath)

	resolveEnv(&AuthServerConfig)
	setDefaultValues()
}

func setDefaultValues() {
	if AuthServerConfig.Host == "" {
		AuthServerConfig.Host = "localhost"
		slog.Debug("set default listening host", "host", AuthServerConfig.Host)
	}
	if AuthServerConfig.Port == 0 {
		AuthServerConfig.Port = 8080
		slog.Debug("set default listening port", "port", AuthServerConfig.Port)
	}
	if AuthServerConfig.Redis.Host == "" {
		AuthServerConfig.Redis.Host = "localhost"
		slog.Debug("set default redis host", "host", AuthServerConfig.Redis.Host)
	}
	if AuthServerConfig.Redis.Port == "" {
		AuthServerConfig.Redis.Port = "6379"
		slog.Debug("set default redis port", "port", AuthServerConfig.Redis.Port)
	}
	if AuthServerConfig.DB == "" {
		// Default PostgreSQL DSN (disable ssl for local dev)
		AuthServerConfig.DB = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
		slog.Debug("set default db address", "dsn", AuthServerConfig.DB)
	}
	if len(AuthServerConfig.JWT) == 0 {
		keySize := 2048
		alg := "RS256"
		privateKey, err := rsa.GenerateKey(rand.Reader, keySize)
		if err != nil {
			slog.Error("failed to generate RSA keys", "err", err)
			panic("failed to generate RSA keys")
		}
		privateKeyPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
		})

		slog.Info("Generated RSA key pair", "algorithm", alg, "key_size", keySize)
		AuthServerConfig.JWT = []JWTConfig{
			{
				Kid: "default",
				Alg: alg,
				Sec: string(privateKeyPEM),
			},
		}
	}
	setAdminDefaults()
	applyAdminEnvOverrides()

	if AuthServerConfig.Cloudflare.Turnstile.Key == "" || AuthServerConfig.Cloudflare.Turnstile.Secret == "" {
		slog.Error("Cloudflare Turnstile is not configured")
		panic("Cloudflare Turnstile un-config")
	}
}

func setAdminDefaults() {
	issuerHost := AuthServerConfig.Host
	if issuerHost == "" || issuerHost == "0.0.0.0" || issuerHost == "::" {
		issuerHost = "localhost"
	}
	if AuthServerConfig.Issuer == "" {
		AuthServerConfig.Issuer = fmt.Sprintf("http://%s:%d", issuerHost, AuthServerConfig.Port)
	}
	if AuthServerConfig.Admin.ClientID == 0 {
		// The administrator SPA has historically used the numeric client id 2.
		// It remains a BIGINT (no string client key); the server owns this row.
		AuthServerConfig.Admin.ClientID = 2
	}
	if AuthServerConfig.Admin.DisplayName == "" {
		AuthServerConfig.Admin.DisplayName = "Auth Server Admin"
	}
	if len(AuthServerConfig.Admin.RedirectURIs) == 0 {
		AuthServerConfig.Admin.RedirectURIs = []string{
			"http://localhost:5173/admin-page/callback",
			"http://127.0.0.1:5173/admin-page/callback",
			"http://localhost:8080/admin-page/callback",
			"http://127.0.0.1:8080/admin-page/callback",
		}
	}
	if len(AuthServerConfig.Admin.Scopes) == 0 {
		AuthServerConfig.Admin.Scopes = []string{"manage:system", "manage:clients"}
	}
	if len(AuthServerConfig.Admin.GrantTypes) == 0 {
		AuthServerConfig.Admin.GrantTypes = []string{"authorization_code", "refresh_token"}
	}
}

func applyAdminEnvOverrides() {
	if v := strings.TrimSpace(os.Getenv("OAUTH_ISSUER")); v != "" {
		AuthServerConfig.Issuer = strings.TrimRight(v, "/")
	}
	if v := strings.TrimSpace(os.Getenv("ADMIN_CLIENT_ID")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			slog.Error("invalid ADMIN_CLIENT_ID", "value", v)
			panic("ADMIN_CLIENT_ID must be a positive integer")
		}
		AuthServerConfig.Admin.ClientID = id
	}
	if v := strings.TrimSpace(os.Getenv("ADMIN_CLIENT_DISPLAY_NAME")); v != "" {
		AuthServerConfig.Admin.DisplayName = v
	}
	if v := strings.TrimSpace(os.Getenv("ADMIN_CLIENT_REDIRECT_URIS")); v != "" {
		AuthServerConfig.Admin.RedirectURIs = splitCSV(v)
	}
	if v := strings.TrimSpace(os.Getenv("ADMIN_CLIENT_SCOPES")); v != "" {
		AuthServerConfig.Admin.Scopes = splitCSV(v)
	}
	if v := strings.TrimSpace(os.Getenv("ADMIN_CLIENT_GRANT_TYPES")); v != "" {
		AuthServerConfig.Admin.GrantTypes = splitCSV(v)
	}
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func resolveEnv(cfg *Config) {
	cfgV := reflect.ValueOf(cfg).Elem()
	resolveStruct(cfgV)
}

func resolveStruct(v reflect.Value) {
	t := v.Type()
	for i := range v.NumField() {
		field := v.Field(i)
		fieldType := t.Field(i)

		switch field.Kind() {
		case reflect.String:
			resolveString(field, fieldType.Name)
		case reflect.Slice:
			for j := 0; j < field.Len(); j++ {
				resolveStruct(field.Index(j))
			}
		case reflect.Struct:
			resolveStruct(field)
		default:
		}
	}
}

func resolveString(v reflect.Value, fieldName string) {
	val := v.String()
	re := regexp.MustCompile(`\$\{(.+?)(?::(.+?))?}`)
	matches := re.FindStringSubmatch(val)

	if len(matches) > 1 {
		envKey := matches[1]
		defaultValue := ""
		if len(matches) == 3 {
			defaultValue = matches[2]
		}

		envVal, found := os.LookupEnv(envKey)
		if !found {
			if defaultValue == "" {
				slog.Error("required environment variable not set", "field", fieldName, "env", envKey)
				panic(fmt.Sprintf("Environment variable for %s (%s) is not set and no default value is provided", fieldName, envKey))
			}
			envVal = defaultValue
		}
		v.SetString(envVal)
	}
}
