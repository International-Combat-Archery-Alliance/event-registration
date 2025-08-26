package api

import (
	"context"
	"fmt"

	"google.golang.org/api/idtoken"
)

const (
	adminScope = "admin"
)

var (
	scopeValidators map[string]func(jwt *idtoken.Payload) error = map[string]func(jwt *idtoken.Payload) error{
		"admin": func(jwt *idtoken.Payload) error {
			org, ok := jwt.Claims["hd"]
			if !ok {
				return fmt.Errorf("hd claim not in JWT")
			}
			if org != "icaa.world" {
				return fmt.Errorf("user is not an admin")
			}

			return nil
		},
	}
)

func (a *API) validateGoogleOauthToken(ctx context.Context, token string, scopes []string) (*idtoken.Payload, error) {
	jwt, err := a.googleIdVerifier.Validate(ctx, token, googleAudience)
	if err != nil {
		return nil, err
	}

	for _, scope := range scopes {
		validator, ok := scopeValidators[scope]
		if !ok {
			return nil, fmt.Errorf("unknown scope: %q", scope)
		}

		err = validator(jwt)
		if err != nil {
			return nil, fmt.Errorf("user does not have scope %q", scope)
		}
	}

	return jwt, nil
}

