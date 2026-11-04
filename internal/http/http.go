package http

import (
	"auth-server/internal"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
)

type TurnstileResp struct {
	Success     bool      `json:"success"`
	ErrorCodes  []string  `json:"error-codes"`
	ChallengeTs time.Time `json:"challenge_ts"`
	Hostname    string    `json:"hostname"`
}

var turnstileClient = resty.New().SetTimeout(10 * time.Second)

func verifyRequest(ctx context.Context, token, ip string) error {
	response, err := turnstileClient.R().SetContext(ctx).SetFormData(map[string]string{
		"secret":   internal.AuthServerConfig.Cloudflare.Turnstile.Secret,
		"response": token,
		"remoteip": ip,
	}).Post("https://challenges.cloudflare.com/turnstile/v0/siteverify")
	if err != nil {
		return err
	}
	re := TurnstileResp{}
	if err = json.Unmarshal(response.Body(), &re); err != nil {
		return err
	}
	if !re.Success {
		return fmt.Errorf("%v", re.ErrorCodes)
	}
	return nil
}

func responseJson(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	if code == 0 {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusInternalServerError)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "data": data})
}
