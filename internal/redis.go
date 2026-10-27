package internal

import (
	"crypto/tls"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"
)

var Rdb *redis.Client

func init() {
	options := &redis.Options{
		Addr:     fmt.Sprintf("%s:%s", AuthServerConfig.Redis.Host, AuthServerConfig.Redis.Port),
		Username: AuthServerConfig.Redis.Username,
		Password: AuthServerConfig.Redis.Password,
		DB:       0,
	}
	if AuthServerConfig.Redis.TLS {
		options.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	Rdb = redis.NewClient(options)
	slog.Debug("redis client initialized", "host", AuthServerConfig.Redis.Host, "port", AuthServerConfig.Redis.Port)
}
