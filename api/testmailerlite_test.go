package api

import (
	"context"
	"testing"

	"github.com/International-Combat-Archery-Alliance/email"
	"github.com/oapi-codegen/runtime/types"
)

func TestPostEventsV1AdminTestMailerlite_IndividualSuccess(t *testing.T) {
	subMgr := &mockSubscriberManager{}
	api := NewAPI(&mockDB{}, noopLogger, LOCAL, newTestTokenService(), &mockCaptchaValidator{}, &mockEmailSender{}, subMgr, &mockCheckoutManager{}, func(context.Context) error { return nil })

	emails := []types.Email{types.Email("jane.archer@example.com"), types.Email("john.doe@example.com")}

	resp, err := api.PostEventsV1AdminTestMailerlite(context.Background(), PostEventsV1AdminTestMailerliteRequestObject{
		Body: &PostEventsV1AdminTestMailerliteJSONRequestBody{
			RegistrationType: ByIndividual,
			Emails:           emails,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result, ok := resp.(PostEventsV1AdminTestMailerlite200JSONResponse)
	if !ok {
		t.Fatalf("expected 200 response, got %T", resp)
	}
	if result.GroupId == nil || *result.GroupId != "mock-group-id" {
		t.Errorf("expected groupId 'mock-group-id', got %v", result.GroupId)
	}
	if result.GroupName == nil || *result.GroupName != "MailerLite Test Group" {
		t.Errorf("expected groupName 'MailerLite Test Group', got %v", result.GroupName)
	}
}

func TestPostEventsV1AdminTestMailerlite_CustomGroupName(t *testing.T) {
	subMgr := &mockSubscriberManager{}
	api := NewAPI(&mockDB{}, noopLogger, LOCAL, newTestTokenService(), &mockCaptchaValidator{}, &mockEmailSender{}, subMgr, &mockCheckoutManager{}, func(context.Context) error { return nil })

	customName := "My Custom Group"
	emails := []types.Email{types.Email("test@example.com")}

	resp, err := api.PostEventsV1AdminTestMailerlite(context.Background(), PostEventsV1AdminTestMailerliteRequestObject{
		Body: &PostEventsV1AdminTestMailerliteJSONRequestBody{
			RegistrationType: ByIndividual,
			GroupName:        &customName,
			Emails:           emails,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result, ok := resp.(PostEventsV1AdminTestMailerlite200JSONResponse)
	if !ok {
		t.Fatalf("expected 200 response, got %T", resp)
	}
	if result.GroupName == nil || *result.GroupName != customName {
		t.Errorf("expected groupName %q, got %v", customName, result.GroupName)
	}
}

func TestPostEventsV1AdminTestMailerlite_TeamSuccess(t *testing.T) {
	subMgr := &mockSubscriberManager{}
	api := NewAPI(&mockDB{}, noopLogger, LOCAL, newTestTokenService(), &mockCaptchaValidator{}, &mockEmailSender{}, subMgr, &mockCheckoutManager{}, func(context.Context) error { return nil })

	teamName := "Test Team"
	emails := []types.Email{
		types.Email("captain@example.com"),
		types.Email("player1@example.com"),
		types.Email("player2@example.com"),
	}

	resp, err := api.PostEventsV1AdminTestMailerlite(context.Background(), PostEventsV1AdminTestMailerliteRequestObject{
		Body: &PostEventsV1AdminTestMailerliteJSONRequestBody{
			RegistrationType: ByTeam,
			TeamName:         &teamName,
			Emails:           emails,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(PostEventsV1AdminTestMailerlite200JSONResponse); !ok {
		t.Fatalf("expected 200 response, got %T", resp)
	}
}

func TestPostEventsV1AdminTestMailerlite_TeamMissingTeamName(t *testing.T) {
	subMgr := &mockSubscriberManager{}
	api := NewAPI(&mockDB{}, noopLogger, LOCAL, newTestTokenService(), &mockCaptchaValidator{}, &mockEmailSender{}, subMgr, &mockCheckoutManager{}, func(context.Context) error { return nil })

	emails := []types.Email{types.Email("captain@example.com")}

	resp, err := api.PostEventsV1AdminTestMailerlite(context.Background(), PostEventsV1AdminTestMailerliteRequestObject{
		Body: &PostEventsV1AdminTestMailerliteJSONRequestBody{
			RegistrationType: ByTeam,
			Emails:           emails,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(PostEventsV1AdminTestMailerlite400JSONResponse); !ok {
		t.Errorf("expected 400 response, got %T", resp)
	}
}

func TestPostEventsV1AdminTestMailerlite_CreateGroupFailure(t *testing.T) {
	subMgr := &mockSubscriberManager{
		CreateGroupFunc: func(ctx context.Context, name string) (string, error) {
			return "", email.NewServiceError("api error", nil)
		},
	}
	api := NewAPI(&mockDB{}, noopLogger, LOCAL, newTestTokenService(), &mockCaptchaValidator{}, &mockEmailSender{}, subMgr, &mockCheckoutManager{}, func(context.Context) error { return nil })

	emails := []types.Email{types.Email("test@example.com")}

	resp, err := api.PostEventsV1AdminTestMailerlite(context.Background(), PostEventsV1AdminTestMailerliteRequestObject{
		Body: &PostEventsV1AdminTestMailerliteJSONRequestBody{
			RegistrationType: ByIndividual,
			Emails:           emails,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(PostEventsV1AdminTestMailerlite500JSONResponse); !ok {
		t.Errorf("expected 500 response, got %T", resp)
	}
}


