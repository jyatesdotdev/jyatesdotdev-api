package email

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	sestypes "github.com/aws/aws-sdk-go-v2/service/ses/types"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

type Service interface {
	SendAdminNotification(ctx context.Context, subject, body string) error
	SendContactEmail(ctx context.Context, name, replyTo, message string) error
}

type SESClient struct {
	api       *sesv2.Client
	v1api     *ses.Client
	fromEmail string
	toEmail   string
}

func NewSESClient(ctx context.Context) (*SESClient, error) {
	fromEmail := os.Getenv("SES_FROM_EMAIL")
	toEmail := os.Getenv("SES_ADMIN_EMAIL")

	if fromEmail == "" || toEmail == "" {
		return &SESClient{}, nil
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	endpoint := os.Getenv("SES_ENDPOINT")
	client := &SESClient{
		fromEmail: fromEmail,
		toEmail:   toEmail,
	}

	if endpoint != "" {
		// LocalStack: use SES v1 (v2 is pro-only)
		client.v1api = ses.NewFromConfig(cfg, func(o *ses.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})
	} else {
		client.api = sesv2.NewFromConfig(cfg)
	}

	return client, nil
}

func (s *SESClient) sendEmail(ctx context.Context, subject, body string, replyTo []string) error {
	if s.v1api != nil {
		input := &ses.SendEmailInput{
			Source:      aws.String(s.fromEmail),
			Destination: &sestypes.Destination{ToAddresses: []string{s.toEmail}},
			Message: &sestypes.Message{
				Subject: &sestypes.Content{Data: aws.String(subject), Charset: aws.String("UTF-8")},
				Body:    &sestypes.Body{Text: &sestypes.Content{Data: aws.String(body), Charset: aws.String("UTF-8")}},
			},
			ReplyToAddresses: replyTo,
		}
		_, err := s.v1api.SendEmail(ctx, input)
		return err
	}

	if s.api != nil {
		input := &sesv2.SendEmailInput{
			FromEmailAddress: aws.String(s.fromEmail),
			Destination:      &types.Destination{ToAddresses: []string{s.toEmail}},
			ReplyToAddresses: replyTo,
			Content: &types.EmailContent{
				Simple: &types.Message{
					Subject: &types.Content{Data: aws.String(subject), Charset: aws.String("UTF-8")},
					Body:    &types.Body{Text: &types.Content{Data: aws.String(body), Charset: aws.String("UTF-8")}},
				},
			},
		}
		_, err := s.api.SendEmail(ctx, input)
		return err
	}

	fmt.Println("SES not configured, skipping email:", subject)
	return nil
}

func (s *SESClient) SendAdminNotification(ctx context.Context, subject, body string) error {
	return s.sendEmail(ctx, subject, body, nil)
}

func (s *SESClient) SendContactEmail(ctx context.Context, name, replyTo, message string) error {
	subject := fmt.Sprintf("New Contact Form Submission from %s", name)
	body := fmt.Sprintf("Name: %s\nEmail: %s\n\nMessage:\n%s", name, replyTo, message)
	return s.sendEmail(ctx, subject, body, []string{replyTo})
}
