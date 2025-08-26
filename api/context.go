package api

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"google.golang.org/api/idtoken"
)

const (
	ctxRequestIdKey = "REQUEST_ID"
	ctxLoggerKey    = "LOGGER"
	ctxJWTKey       = "JWT"
)

func ctxWithRequestId(ctx context.Context, requestId uuid.UUID) context.Context {
	return context.WithValue(ctx, ctxRequestIdKey, requestId)
}

func getRequestIdFromCtx(ctx context.Context) uuid.UUID {
	return ctx.Value(ctxRequestIdKey).(uuid.UUID)
}

func ctxWithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxLoggerKey, logger)
}

func getLoggerFromCtx(ctx context.Context) *slog.Logger {
	return ctx.Value(ctxLoggerKey).(*slog.Logger)
}

func ctxWithJWT(ctx context.Context, jwt *idtoken.Payload) context.Context {
	return context.WithValue(ctx, ctxJWTKey, jwt)
}

func getJWTFromCtx(ctx context.Context) *idtoken.Payload {
	return ctx.Value(ctxJWTKey).(*idtoken.Payload)
}
