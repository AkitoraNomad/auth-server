package util

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

const telegramAuthMaxAge = 5 * time.Minute
const telegramAuthClockSkew = 60 * time.Second

// TelegramAuth is the verified, typed subset of Telegram Login Widget data.
// It intentionally lives outside model to keep validation free of model/store
// dependencies.
type TelegramAuth struct {
	ID        int64
	FirstName string
	LastName  string
	Username  string
	PhotoURL  string
	AuthDate  int64
}

// ParseTelegramCredential validates the Telegram Login Widget HMAC, required
// fields, hash type and auth_date freshness. It fails closed on malformed data.
func ParseTelegramCredential(jsonData json.RawMessage, botTokenDigest []byte, now time.Time) (*TelegramAuth, error) {
	if len(botTokenDigest) == 0 {
		return nil, errors.New("empty telegram bot token digest")
	}
	dec := json.NewDecoder(bytes.NewReader(jsonData))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode telegram data: %w", err)
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing telegram data")
	}
	if raw == nil {
		return nil, errors.New("telegram data is not an object")
	}
	hashValue, ok := raw["hash"]
	if !ok {
		return nil, errors.New("telegram hash missing")
	}
	hash, ok := hashValue.(string)
	if !ok || len(hash) != sha256.Size*2 {
		return nil, errors.New("telegram hash is not a valid string")
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return nil, errors.New("telegram hash is not hex")
	}

	pairs := make([]string, 0, len(raw)-1)
	for key, value := range raw {
		if key == "hash" {
			continue
		}
		switch v := value.(type) {
		case string:
			pairs = append(pairs, key+"="+v)
		case json.Number:
			if strings.ContainsAny(v.String(), ".eE") {
				return nil, errors.New("telegram numeric field is not an integer")
			}
			pairs = append(pairs, key+"="+v.String())
		default:
			return nil, errors.New("unsupported telegram field type")
		}
	}
	sort.Strings(pairs)
	mac := hmac.New(sha256.New, botTokenDigest)
	_, _ = mac.Write([]byte(strings.Join(pairs, "\n")))
	expected := hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(expected), []byte(hash)) != 1 {
		return nil, errors.New("telegram hash mismatch")
	}

	tg := &TelegramAuth{}
	if err := unmarshalInt64(raw["id"], &tg.ID); err != nil || tg.ID <= 0 {
		return nil, errors.New("telegram id missing or invalid")
	}
	if err := unmarshalInt64(raw["auth_date"], &tg.AuthDate); err != nil || tg.AuthDate <= 0 {
		return nil, errors.New("telegram auth_date missing or invalid")
	}
	if err := unmarshalString(raw["first_name"], &tg.FirstName); err != nil {
		return nil, err
	}
	if err := unmarshalString(raw["last_name"], &tg.LastName); err != nil {
		return nil, err
	}
	if err := unmarshalString(raw["username"], &tg.Username); err != nil {
		return nil, err
	}
	if err := unmarshalString(raw["photo_url"], &tg.PhotoURL); err != nil {
		return nil, err
	}
	authTime := time.Unix(tg.AuthDate, 0)
	if authTime.Before(now.Add(-telegramAuthMaxAge)) {
		return nil, errors.New("telegram auth_date is too old")
	}
	if authTime.After(now.Add(telegramAuthClockSkew)) {
		return nil, errors.New("telegram auth_date is in the future")
	}
	return tg, nil
}

func unmarshalInt64(v any, dst *int64) error {
	n, ok := v.(json.Number)
	if !ok {
		return errors.New("telegram integer field has wrong type")
	}
	i, err := strconv.ParseInt(n.String(), 10, 64)
	if err != nil {
		return errors.New("telegram integer field is invalid")
	}
	*dst = i
	return nil
}

func unmarshalString(v any, dst *string) error {
	if v == nil {
		return nil
	}
	s, ok := v.(string)
	if !ok {
		return errors.New("telegram string field has wrong type")
	}
	*dst = s
	return nil
}

// ValidateTelegramCredential is kept as a boolean convenience wrapper.
func ValidateTelegramCredential(jsonData json.RawMessage, botTokenDigest []byte) bool {
	_, err := ParseTelegramCredential(jsonData, botTokenDigest, time.Now())
	return err == nil
}
