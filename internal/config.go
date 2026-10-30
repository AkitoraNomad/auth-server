package internal

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
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
	TrustedProxies []string `yaml:"trusted-proxies"`
	Host           string   `yaml:"host"`
	Port           int      `yaml:"port"`
	Redis          struct {
		Host     string `yaml:"host"`
		Port     string `yaml:"port"`
		Username string `yaml:"username"`
		Password string `yaml:"password"`
		TLS      bool   `yaml:"tls"`
	} `yaml:"redis"`
	DB     string      `yaml:"db"`
	Issuer string      `yaml:"issuer"`
	JWT    []JWTConfig `yaml:"jwt"`
	Admin  struct {
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

// LoadConfig reads and validates configuration without opening external services.
func LoadConfig(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	if err := resolveEnv(&cfg); err != nil {
		return cfg, err
	}
	if err := cfg.setDefaultValues(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (cfg *Config) setDefaultValues() error {
	if cfg.Host == "" {
		cfg.Host = "localhost"
		slog.Debug("set default listening host", "host", cfg.Host)
	}
	if cfg.Port == 0 {
		cfg.Port = 8080
		slog.Debug("set default listening port", "port", cfg.Port)
	}
	if cfg.Redis.Host == "" {
		cfg.Redis.Host = "localhost"
		slog.Debug("set default redis host", "host", cfg.Redis.Host)
	}
	if cfg.Redis.Port == "" {
		cfg.Redis.Port = "6379"
		slog.Debug("set default redis port", "port", cfg.Redis.Port)
	}
	if cfg.DB == "" {
		// Default PostgreSQL DSN (disable ssl for local dev)
		cfg.DB = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
		slog.Debug("set default local database address")
	}
	if len(cfg.JWT) == 0 {
		keySize := 2048
		alg := "RS256"
		privateKey, err := rsa.GenerateKey(rand.Reader, keySize)
		if err != nil {
			slog.Error("failed to generate RSA keys", "err", err)
			return fmt.Errorf("generate RSA keys: %w", err)
		}
		privateKeyPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
		})

		slog.Info("Generated RSA key pair", "algorithm", alg, "key_size", keySize)
		cfg.JWT = []JWTConfig{
			{
				Kid: "default",
				Alg: alg,
				Sec: string(privateKeyPEM),
			},
		}
	}
	cfg.setAdminDefaults()
	if err := cfg.applyAdminEnvOverrides(); err != nil {
		return err
	}

	if cfg.Cloudflare.Turnstile.Key == "" || cfg.Cloudflare.Turnstile.Secret == "" {
		slog.Error("Cloudflare Turnstile is not configured")
		return fmt.Errorf("Cloudflare Turnstile is not configured")
	}
	return nil
}

func (cfg *Config) setAdminDefaults() {
	issuerHost := cfg.Host
	if issuerHost == "" || issuerHost == "0.0.0.0" || issuerHost == "::" {
		issuerHost = "localhost"
	}
	if cfg.Issuer == "" {
		cfg.Issuer = fmt.Sprintf("http://%s:%d", issuerHost, cfg.Port)
	}
	if cfg.Admin.ClientID == 0 {
		// The administrator SPA has historically used the numeric client id 2.
		// It remains a BIGINT (no string client key); the server owns this row.
		cfg.Admin.ClientID = 2
	}
	if cfg.Admin.DisplayName == "" {
		cfg.Admin.DisplayName = "Auth Server Admin"
	}
	if len(cfg.Admin.RedirectURIs) == 0 {
		cfg.Admin.RedirectURIs = []string{
			"http://localhost:5173/admin-page/callback",
			"http://127.0.0.1:5173/admin-page/callback",
			"http://localhost:8080/admin-page/callback",
			"http://127.0.0.1:8080/admin-page/callback",
		}
	}
	if len(cfg.Admin.Scopes) == 0 {
		cfg.Admin.Scopes = []string{"manage:system", "manage:clients"}
	}
	if len(cfg.Admin.GrantTypes) == 0 {
		cfg.Admin.GrantTypes = []string{"authorization_code", "refresh_token"}
	}
}

func (cfg *Config) applyAdminEnvOverrides() error {
	if v := strings.TrimSpace(os.Getenv("OAUTH_ISSUER")); v != "" {
		cfg.Issuer = strings.TrimRight(v, "/")
	}
	if v := strings.TrimSpace(os.Getenv("ADMIN_CLIENT_ID")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			slog.Error("invalid ADMIN_CLIENT_ID", "value", v)
			return fmt.Errorf("ADMIN_CLIENT_ID must be a positive integer")
		}
		cfg.Admin.ClientID = id
	}
	if v := strings.TrimSpace(os.Getenv("ADMIN_CLIENT_DISPLAY_NAME")); v != "" {
		cfg.Admin.DisplayName = v
	}
	if v := strings.TrimSpace(os.Getenv("ADMIN_CLIENT_REDIRECT_URIS")); v != "" {
		cfg.Admin.RedirectURIs = splitCSV(v)
	}
	if v := strings.TrimSpace(os.Getenv("ADMIN_CLIENT_SCOPES")); v != "" {
		cfg.Admin.Scopes = splitCSV(v)
	}
	if v := strings.TrimSpace(os.Getenv("ADMIN_CLIENT_GRANT_TYPES")); v != "" {
		cfg.Admin.GrantTypes = splitCSV(v)
	}
	return nil
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

func resolveEnv(cfg *Config) error {
	cfgV := reflect.ValueOf(cfg).Elem()
	return resolveStruct(cfgV)
}

func resolveStruct(v reflect.Value) error {
	t := v.Type()
	for i := range v.NumField() {
		field := v.Field(i)
		fieldType := t.Field(i)

		switch field.Kind() {
		case reflect.String:
			if err := resolveString(field, fieldType.Name); err != nil {
				return err
			}
		case reflect.Slice:
			for j := 0; j < field.Len(); j++ {
				element := field.Index(j)
				if element.Kind() == reflect.Struct {
					if err := resolveStruct(element); err != nil {
						return err
					}
				}
				if element.Kind() == reflect.String {
					if err := resolveString(element, fieldType.Name); err != nil {
						return err
					}
				}
			}
		case reflect.Struct:
			if err := resolveStruct(field); err != nil {
				return err
			}
		default:
		}
	}
	return nil
}

var envPattern = regexp.MustCompile(`\$\{(.+?)(?::(.+?))?}`)

func resolveString(v reflect.Value, fieldName string) error {
	val := v.String()
	matches := envPattern.FindStringSubmatch(val)

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
				return fmt.Errorf("environment variable for %s (%s) is not set and no default value is provided", fieldName, envKey)
			}
			envVal = defaultValue
		}
		v.SetString(envVal)
	}
	return nil
}
