package email

import (
	"context"
	"os"
	"testing"
)

func TestNewSESClient_Dummy(t *testing.T) {
	// Ensure env vars are empty
	os.Setenv("SES_FROM_EMAIL", "")
	os.Setenv("SES_ADMIN_EMAIL", "")

	client, err := NewSESClient(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if client.api != nil {
		t.Errorf("expected dummy client with nil api, got %v", client.api)
	}

	// Should not panic or error when sending with dummy client
	err = client.SendAdminNotification(context.Background(), "test", "test")
	if err != nil {
		t.Errorf("expected no error sending admin notification with dummy client, got %v", err)
	}

	err = client.SendContactEmail(context.Background(), "test", "test@example.com", "test")
	if err != nil {
		t.Errorf("expected no error sending contact email with dummy client, got %v", err)
	}
}
