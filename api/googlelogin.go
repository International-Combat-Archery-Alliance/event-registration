package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

const (
	googleAudience         = "1008624351875-q36btbijttq83bogn9f8a4srgji0g3qg.apps.googleusercontent.com"
	googleAuthJWTCookieKey = "GOOGLE_AUTH_JWT"
)

func (a *API) PostGoogleLogin(ctx context.Context, request PostGoogleLoginRequestObject) (PostGoogleLoginResponseObject, error) {
	logger := getLoggerFromCtx(ctx)

	jwtPayload, err := a.googleIdVerifier.Validate(ctx, request.Body.GoogleJWT, googleAudience)
	if err != nil {
		return PostGoogleLogin401JSONResponse{
			Message: "Invalid JWT",
			Code:    AuthError,
		}, nil
	}

	logger.Info("successful login", slog.Any("email", jwtPayload.Claims["email"]))

	cookie := &http.Cookie{
		Name:     googleAuthJWTCookieKey,
		Value:    request.Body.GoogleJWT,
		Expires:  time.Unix(jwtPayload.Expires, 0),
		Domain:   ".icaa.world",
		Path:     "/",
		HttpOnly: true,
		Secure:   a.env == PROD,
		SameSite: http.SameSiteStrictMode,
	}

	return PostGoogleLogin200Response{
		Headers: PostGoogleLogin200ResponseHeaders{
			SetCookie: cookie.String(),
		},
	}, nil
}
