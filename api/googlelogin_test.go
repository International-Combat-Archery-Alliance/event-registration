package api

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/api/idtoken"
)

type mockGoogleIdVerifier struct {
	ValidateFunc func(ctx context.Context, idToken, audience string) (*idtoken.Payload, error)
}

func (m *mockGoogleIdVerifier) Validate(ctx context.Context, idToken, audience string) (*idtoken.Payload, error) {
	return m.ValidateFunc(ctx, idToken, audience)
}

func TestPostGoogleLogin(t *testing.T) {
	t.Run("success with valid JWT", func(t *testing.T) {
		validJWT := "valid.jwt.token"
		validEmail := "test@example.com"
		expiresTime := time.Now().Add(time.Hour).Unix()

		mockVerifier := &mockGoogleIdVerifier{
			ValidateFunc: func(ctx context.Context, idToken, audience string) (*idtoken.Payload, error) {
				assert.Equal(t, validJWT, idToken)
				assert.Equal(t, googleAudience, audience)
				return &idtoken.Payload{
					Expires: expiresTime,
					Claims: map[string]any{
						"email": validEmail,
					},
				}, nil
			},
		}

		api := &API{
			db:               &mockDB{},
			logger:           noopLogger,
			env:              LOCAL,
			googleIdVerifier: mockVerifier,
		}

		req := PostGoogleLoginRequestObject{
			Body: &PostGoogleLoginJSONRequestBody{
				GoogleJWT: validJWT,
			},
		}

		resp, err := api.PostGoogleLogin(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostGoogleLogin200Response:
			assert.Contains(t, r.Headers.SetCookie, googleAuthJWTCookieKey+"="+validJWT)
			assert.Contains(t, r.Headers.SetCookie, "Domain=icaa.world")
			assert.Contains(t, r.Headers.SetCookie, "Path=/")
			assert.Contains(t, r.Headers.SetCookie, "HttpOnly")
			assert.Contains(t, r.Headers.SetCookie, "SameSite=Strict")
			// For LOCAL env, Secure should not be set
			assert.NotContains(t, r.Headers.SetCookie, "Secure")
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("success with PROD environment sets secure cookie", func(t *testing.T) {
		validJWT := "valid.jwt.token"
		validEmail := "test@example.com"
		expiresTime := time.Now().Add(time.Hour).Unix()

		mockVerifier := &mockGoogleIdVerifier{
			ValidateFunc: func(ctx context.Context, idToken, audience string) (*idtoken.Payload, error) {
				return &idtoken.Payload{
					Expires: expiresTime,
					Claims: map[string]any{
						"email": validEmail,
					},
				}, nil
			},
		}

		api := &API{
			db:               &mockDB{},
			logger:           noopLogger,
			env:              PROD,
			googleIdVerifier: mockVerifier,
		}

		req := PostGoogleLoginRequestObject{
			Body: &PostGoogleLoginJSONRequestBody{
				GoogleJWT: validJWT,
			},
		}

		resp, err := api.PostGoogleLogin(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostGoogleLogin200Response:
			assert.Contains(t, r.Headers.SetCookie, "Secure")
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("invalid JWT returns 401", func(t *testing.T) {
		invalidJWT := "invalid.jwt.token"

		mockVerifier := &mockGoogleIdVerifier{
			ValidateFunc: func(ctx context.Context, idToken, audience string) (*idtoken.Payload, error) {
				assert.Equal(t, invalidJWT, idToken)
				assert.Equal(t, googleAudience, audience)
				return nil, errors.New("invalid token")
			},
		}

		api := &API{
			db:               &mockDB{},
			logger:           noopLogger,
			env:              LOCAL,
			googleIdVerifier: mockVerifier,
		}

		req := PostGoogleLoginRequestObject{
			Body: &PostGoogleLoginJSONRequestBody{
				GoogleJWT: invalidJWT,
			},
		}

		resp, err := api.PostGoogleLogin(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostGoogleLogin401JSONResponse:
			assert.Equal(t, AuthError, r.Code)
			assert.Equal(t, "Invalid JWT", r.Message)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("expired JWT returns 401", func(t *testing.T) {
		expiredJWT := "expired.jwt.token"

		mockVerifier := &mockGoogleIdVerifier{
			ValidateFunc: func(ctx context.Context, idToken, audience string) (*idtoken.Payload, error) {
				return nil, errors.New("token is expired")
			},
		}

		api := &API{
			db:               &mockDB{},
			logger:           noopLogger,
			env:              LOCAL,
			googleIdVerifier: mockVerifier,
		}

		req := PostGoogleLoginRequestObject{
			Body: &PostGoogleLoginJSONRequestBody{
				GoogleJWT: expiredJWT,
			},
		}

		resp, err := api.PostGoogleLogin(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostGoogleLogin401JSONResponse:
			assert.Equal(t, AuthError, r.Code)
			assert.Equal(t, "Invalid JWT", r.Message)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("wrong audience JWT returns 401", func(t *testing.T) {
		wrongAudienceJWT := "wrong.audience.token"

		mockVerifier := &mockGoogleIdVerifier{
			ValidateFunc: func(ctx context.Context, idToken, audience string) (*idtoken.Payload, error) {
				assert.Equal(t, googleAudience, audience)
				return nil, errors.New("audience mismatch")
			},
		}

		api := &API{
			db:               &mockDB{},
			logger:           noopLogger,
			env:              LOCAL,
			googleIdVerifier: mockVerifier,
		}

		req := PostGoogleLoginRequestObject{
			Body: &PostGoogleLoginJSONRequestBody{
				GoogleJWT: wrongAudienceJWT,
			},
		}

		resp, err := api.PostGoogleLogin(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostGoogleLogin401JSONResponse:
			assert.Equal(t, AuthError, r.Code)
			assert.Equal(t, "Invalid JWT", r.Message)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("cookie expiration matches JWT expiration", func(t *testing.T) {
		validJWT := "valid.jwt.token"
		validEmail := "test@example.com"
		futureTime := time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)
		expiresTime := futureTime.Unix()

		mockVerifier := &mockGoogleIdVerifier{
			ValidateFunc: func(ctx context.Context, idToken, audience string) (*idtoken.Payload, error) {
				return &idtoken.Payload{
					Expires: expiresTime,
					Claims: map[string]any{
						"email": validEmail,
					},
				}, nil
			},
		}

		api := &API{
			db:               &mockDB{},
			logger:           noopLogger,
			env:              LOCAL,
			googleIdVerifier: mockVerifier,
		}

		req := PostGoogleLoginRequestObject{
			Body: &PostGoogleLoginJSONRequestBody{
				GoogleJWT: validJWT,
			},
		}

		resp, err := api.PostGoogleLogin(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostGoogleLogin200Response:
			expectedExpires := futureTime.Format(http.TimeFormat)
			assert.Contains(t, r.Headers.SetCookie, "Expires="+expectedExpires)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}
