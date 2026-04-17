package email

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

type Service interface {
	SendAdminNotification(ctx context.Context, subject, body string) error
	SendContactEmail(ctx context.Context, name, replyTo, message string) error
}

type SESClient struct {
	api       *sesv2.Client
	fromEmail string
	toEmail   string
}

func NewSESClient(ctx context.Context) (*SESClient, error) {
	fromEmail := os.Getenv("SES_FROM_EMAIL")
	toEmail := os.Getenv("SES_ADMIN_EMAIL")

	if fromEmail == "" || toEmail == "" {
		// Return a dummy client if emails aren't configured (e.g., for local dev)
		return &SESClient{}, nil
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	return &SESClient{
		api:       sesv2.NewFromConfig(cfg),
		fromEmail: fromEmail,
		toEmail:   toEmail,
	}, nil
}

func (s *SESClient) SendAdminNotification(ctx context.Context, subject, body string) error {
	if s.api == nil {
		// Skip sending if not configured
		fmt.Println("SES not configured, skipping admin notification:")
		fmt.Println("Subject:", subject)
		fmt.Println("Body:", body)
		return nil
	}

	input := &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(s.fromEmail),
		Destination: &types.Destination{
			ToAddresses: []string{s.toEmail},
		},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{
					Data:    aws.String(subject),
					Charset: aws.String("UTF-8"),
				},
				Body: &types.Body{
					Text: &types.Content{
						Data:    aws.String(body),
						Charset: aws.String("UTF-8"),
					},
				},
			},
		},
	}

	_, err := s.api.SendEmail(ctx, input)
	return err
}

func (s *SESClient) SendContactEmail(ctx context.Context, name, replyTo, message string) error {
	if s.api == nil {
		// Skip sending if not configured
		fmt.Println("SES not configured, skipping contact email:")
		fmt.Println("From:", name, "<"+replyTo+">")
		fmt.Println("Message:", message)
		return nil
	}

	subject := fmt.Sprintf("New Contact Form Submission from %s", name)
	body := fmt.Sprintf("Name: %s\nEmail: %s\n\nMessage:\n%s", name, replyTo, message)

	input := &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(s.fromEmail),
		Destination: &types.Destination{
			ToAddresses: []string{s.toEmail},
		},
		ReplyToAddresses: []string{replyTo},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{
					Data:    aws.String(subject),
					Charset: aws.String("UTF-8"),
				},
				Body: &types.Body{
					Text: &types.Content{
						Data:    aws.String(body),
						Charset: aws.String("UTF-8"),
					},
				},
			},
		},
	}

	_, err := s.api.SendEmail(ctx, input)
	return err
}
