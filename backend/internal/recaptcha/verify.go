package recaptcha

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"
)

type VerifyResponse struct {
	Success     bool      `json:"success"`
	Score       float64   `json:"score"`
	Action      string    `json:"action"`
	ChallengeTS time.Time `json:"challenge_ts"`
	Hostname    string    `json:"hostname"`
	ErrorCodes  []string  `json:"error-codes"`
}

const (
	VerifyURL    = "https://www.google.com/recaptcha/api/siteverify"
	MinScore     = 0.5
	VerifyAction = "like" // Default action for likes
)

func Verify(token string, action string) (bool, error) {
	secretKey := os.Getenv("RECAPTCHA_SECRET_KEY")
	if secretKey == "" {
		// For local testing if secret not provided, we could skip it or fail.
		// For now, fail unless explicitly disabled.
		if os.Getenv("SKIP_RECAPTCHA") == "true" {
			return true, nil
		}
		return false, fmt.Errorf("RECAPTCHA_SECRET_KEY is not set")
	}

	resp, err := http.PostForm(VerifyURL, url.Values{
		"secret":   {secretKey},
		"response": {token},
	})
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var verifyResp VerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&verifyResp); err != nil {
		return false, err
	}

	if !verifyResp.Success {
		return false, nil
	}

	if action != "" && verifyResp.Action != action {
		return false, nil
	}

	if verifyResp.Score < MinScore {
		return false, nil
	}

	return true, nil
}
