package api

import (
	"context"
	"testing"

	"github.com/International-Combat-Archery-Alliance/email"
	"github.com/oapi-codegen/runtime/types"
)

func TestPostEventsV1AdminTestEmail_Success(t *testing.T) {
	api := NewAPI(&mockDB{}, noopLogger, LOCAL, newTestTokenService(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

	email := types.Email("test@example.com")
	resp, err := api.PostEventsV1AdminTestEmail(context.Background(), PostEventsV1AdminTestEmailRequestObject{
		Body: &PostEventsV1AdminTestEmailJSONRequestBody{
			Email: email,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(PostEventsV1AdminTestEmail200Response); !ok {
		t.Errorf("expected 200 response, got %T", resp)
	}
}

type mockFailingEmailSender struct{}

func (m *mockFailingEmailSender) SendEmail(ctx context.Context, e email.Email) error {
	return email.NewServiceError("failed", nil)
}

func TestPostEventsV1AdminTestEmail_SendFailure(t *testing.T) {
	api := NewAPI(&mockDB{}, noopLogger, LOCAL, newTestTokenService(), &mockCaptchaValidator{}, &mockFailingEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

	email := types.Email("test@example.com")
	resp, err := api.PostEventsV1AdminTestEmail(context.Background(), PostEventsV1AdminTestEmailRequestObject{
		Body: &PostEventsV1AdminTestEmailJSONRequestBody{
			Email: email,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(PostEventsV1AdminTestEmail500JSONResponse); !ok {
		t.Errorf("expected 500 response, got %T", resp)
	}
}
