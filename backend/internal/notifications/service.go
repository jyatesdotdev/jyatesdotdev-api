package notifications

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jyates/jyatesdotdev-api/backend/internal/subscriptions"
)

type Mailer interface {
	SendUpdateNotification(
		ctx context.Context,
		to, topic, subject, body, contactListName string,
	) error
}

type DeliveryResult struct {
	Duplicate bool
	Sent      int
	Failed    int
	Skipped   int
}

type Service struct {
	objects         ObjectStore
	deliveries      DeliveryRepository
	contacts        subscriptions.ContactStore
	mailer          Mailer
	contactListName string
}

func NewService(
	objects ObjectStore,
	deliveries DeliveryRepository,
	contacts subscriptions.ContactStore,
	mailer Mailer,
	contactListName string,
) *Service {
	return &Service{
		objects:         objects,
		deliveries:      deliveries,
		contacts:        contacts,
		mailer:          mailer,
		contactListName: contactListName,
	}
}

func (s *Service) Deliver(ctx context.Context, bucket, key string) (DeliveryResult, error) {
	if bucket == "" || !strings.HasPrefix(key, "notification-events/") || !strings.HasSuffix(key, ".json") {
		return DeliveryResult{}, errors.New("unexpected notification object")
	}
	objectID := strings.TrimSuffix(strings.TrimPrefix(key, "notification-events/"), ".json")
	if objectID == "" || strings.Contains(objectID, "/") {
		return DeliveryResult{}, errors.New("unexpected notification object")
	}
	body, err := s.objects.Get(ctx, bucket, key)
	if err != nil {
		return DeliveryResult{}, fmt.Errorf("read notification manifest: %w", err)
	}
	defer body.Close()

	manifest, err := ParseManifest(body)
	if err != nil {
		return DeliveryResult{}, err
	}
	if manifest.ID != objectID {
		return DeliveryResult{}, errors.New("notification object key does not match manifest ID")
	}
	complete, err := s.deliveries.ManifestComplete(ctx, manifest.ID)
	if err != nil {
		return DeliveryResult{}, fmt.Errorf("check notification manifest: %w", err)
	}
	if complete {
		return DeliveryResult{Duplicate: true}, nil
	}

	result := DeliveryResult{}
	var deliveryErr error
	for eventIndex, event := range manifest.Events {
		if err := s.deliverEvent(ctx, manifest.ID, eventIndex, event, &result); err != nil {
			deliveryErr = errors.Join(deliveryErr, err)
		}
	}
	if deliveryErr != nil {
		return result, deliveryErr
	}
	if err := s.deliveries.CompleteManifest(ctx, manifest.ID); err != nil {
		return result, fmt.Errorf("complete notification manifest: %w", err)
	}
	return result, nil
}

func (s *Service) deliverEvent(
	ctx context.Context,
	manifestID string,
	eventIndex int,
	event ContentEvent,
	result *DeliveryResult,
) error {
	nextToken := ""
	var deliveryErr error
	for {
		emails, next, err := s.contacts.ListContacts(ctx, event.Topic, nextToken)
		if err != nil {
			return errors.Join(deliveryErr, fmt.Errorf("list %s subscribers: %w", event.Topic, err))
		}
		for _, email := range emails {
			claim, err := s.deliveries.BeginRecipient(ctx, manifestID, eventIndex, email)
			if err != nil {
				deliveryErr = errors.Join(
					deliveryErr,
					fmt.Errorf("claim notification recipient: %w", err),
				)
				continue
			}
			switch claim.State {
			case RecipientComplete:
				result.Skipped++
				continue
			case RecipientInProgress:
				deliveryErr = errors.Join(
					deliveryErr,
					errors.New("notification recipient is still processing"),
				)
				continue
			case RecipientStarted:
			default:
				deliveryErr = errors.Join(
					deliveryErr,
					errors.New("unknown notification recipient state"),
				)
				continue
			}
			if err := s.mailer.SendUpdateNotification(
				ctx,
				email,
				event.Topic,
				event.Subject(),
				event.Body(),
				s.contactListName,
			); err != nil {
				result.Failed++
				if releaseErr := s.deliveries.ReleaseRecipient(ctx, claim); releaseErr != nil {
					deliveryErr = errors.Join(deliveryErr, fmt.Errorf(
						"send notification: %v; release recipient claim: %w",
						err,
						releaseErr,
					))
					continue
				}
				deliveryErr = errors.Join(deliveryErr, fmt.Errorf("send notification: %w", err))
				continue
			}
			if err := s.deliveries.CompleteRecipient(ctx, claim); err != nil {
				deliveryErr = errors.Join(
					deliveryErr,
					fmt.Errorf("complete notification recipient: %w", err),
				)
				continue
			}
			result.Sent++
		}
		if next == "" {
			return deliveryErr
		}
		nextToken = next
	}
}
