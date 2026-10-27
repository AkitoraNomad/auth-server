package http

import (
	"auth-server/internal"
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
)

const authRateScript = `-- auth-rate-limit-v1
local count = redis.call('incr', KEYS[1])
if count == 1 then redis.call('expire', KEYS[1], 60) end
return count`

func allowAuthAttempt(ctx context.Context, key string, maximum int64) bool {
	if internal.Rdb == nil {
		return false
	}
	sum := sha256.Sum256([]byte(key))
	count, err := internal.Rdb.Eval(ctx, authRateScript, []string{fmt.Sprintf("auth:attempt:%x", sum)}).Int64()
	return err == nil && count <= maximum
}

func authRateMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodPost && (c.Request.URL.Path == internal.PathLogin || c.Request.URL.Path == internal.PathRegistration) {
			if !allowAuthAttempt(c.Request.Context(), "ip:"+getIp(c.Request), 30) {
				c.Header("Retry-After", "60")
				c.AbortWithStatus(http.StatusTooManyRequests)
				return
			}
		}
		c.Next()
	}
}
