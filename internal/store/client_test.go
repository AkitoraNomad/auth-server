package store

import (
	"auth-server/internal/model"
	"context"
	"errors"
	"testing"
)

func TestOrdinaryClientCreationCannotSetInternalKey(t *testing.T) {
	key := model.AdminClientKey
	client := &model.AuthClient{InternalKey: &key}
	if err := (&DbClientStore{}).Create(context.Background(), client); !errors.Is(err, ErrProtectedClient) {
		t.Fatalf("internal client creation was not rejected: %v", err)
	}
}
