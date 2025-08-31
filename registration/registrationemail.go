package registration

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"

	"github.com/International-Combat-Archery-Alliance/email"
	"github.com/International-Combat-Archery-Alliance/event-registration/events"
)

//go:embed templates
var templates embed.FS

func SendRegistrationConfirmationEmail(ctx context.Context, emailSender email.Sender, fromAddress string, reg Registration, event events.Event) error {
	htmlBody, err := makeHtmlBody(event, reg)
	if err != nil {
		return err
	}

	textOnlyBody, err := makeTextOnlyBody(event, reg)
	if err != nil {
		return err
	}

	return emailSender.SendEmail(ctx, email.Email{
		FromAddress: fromAddress,
		ToAddresses: []string{reg.GetEmail()},
		Subject:     fmt.Sprintf("Event signup confirmed - %q", event.Name),
		HTMLBody:    htmlBody,
		TextBody:    textOnlyBody,
	})
}

func makeHtmlBody(event events.Event, reg Registration) (string, error) {
	tmpl, err := template.New("registration-confirmation.tmpl").Funcs(template.FuncMap{
		"add": func(a, b int) int { return a + b },
	}).ParseFS(templates, "templates/registration-confirmation.tmpl")
	if err != nil {
		return "", fmt.Errorf("failed to parse email template: %w", err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, map[string]any{
		"Event":        event,
		"Registration": reg,
	})
	if err != nil {
		return "", fmt.Errorf("failed to execute email template: %w", err)
	}

	return buf.String(), nil
}

func makeTextOnlyBody(event events.Event, reg Registration) (string, error) {
	tmpl, err := template.New("registration-confirmation-textonly.tmpl").Funcs(template.FuncMap{
		"add": func(a, b int) int { return a + b },
	}).ParseFS(templates, "templates/registration-confirmation-textonly.tmpl")
	if err != nil {
		return "", fmt.Errorf("failed to parse email template: %w", err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, map[string]any{
		"Event":        event,
		"Registration": reg,
	})
	if err != nil {
		return "", fmt.Errorf("failed to execute email template: %w", err)
	}

	return buf.String(), nil
}
