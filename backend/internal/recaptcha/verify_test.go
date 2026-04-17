package recaptcha

import (
	"os"
	"testing"
)

func TestVerify_MissingSecret(t *testing.T) {
	os.Setenv("RECAPTCHA_SECRET_KEY", "")
	os.Setenv("SKIP_RECAPTCHA", "false")

	success, err := Verify("dummy_token", "test_action")
	if success {
		t.Errorf("expected failure, got success")
	}
	if err == nil || err.Error() != "RECAPTCHA_SECRET_KEY is not set" {
		t.Errorf("expected RECAPTCHA_SECRET_KEY is not set error, got %v", err)
	}
}

func TestVerify_SkipRecaptcha(t *testing.T) {
	os.Setenv("RECAPTCHA_SECRET_KEY", "")
	os.Setenv("SKIP_RECAPTCHA", "true")

	success, err := Verify("dummy_token", "test_action")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !success {
		t.Errorf("expected success when SKIP_RECAPTCHA is true")
	}
}
