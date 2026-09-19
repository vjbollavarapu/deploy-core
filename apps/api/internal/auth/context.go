package auth

import (
	"context"

	"github.com/google/uuid"
)

type ctxKey int

const (
	userCtxKey ctxKey = iota
	sessionCtxKey
)

func WithUser(ctx context.Context, user User) context.Context {
	return context.WithValue(ctx, userCtxKey, user)
}

func UserFromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userCtxKey).(User)
	return u, ok
}

func WithSessionID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, sessionCtxKey, id)
}

func SessionIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(sessionCtxKey).(uuid.UUID)
	return id, ok
}
