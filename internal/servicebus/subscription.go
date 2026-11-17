package servicebus

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus/admin"
)

const (
	temporarySubscriptionPrefix = "postoffice-"
	// temporarySubscriptionIDBytes makes the name unique per watch: 8 hex digits.
	temporarySubscriptionIDBytes = 4
	// temporarySubscriptionIdleLifetime lets the broker delete a temporary
	// subscription whose watch exited without closing it. Five minutes is
	// the shortest it accepts.
	temporarySubscriptionIdleLifetime = "PT5M"
)

// adminSubscriptions manages temporary subscriptions through the
// administration client.
type adminSubscriptions struct {
	client *admin.Client
}

func newAdminSubscriptions(connectionString string) (adminSubscriptions, error) {
	client, err := admin.NewClientFromConnectionString(connectionString, nil)
	if err != nil {
		return adminSubscriptions{}, fmt.Errorf("failed to create Service Bus administration client: %w", err)
	}
	return adminSubscriptions{client: client}, nil
}

func (s adminSubscriptions) createSubscription(ctx context.Context, topic, subscription string) error {
	idleLifetime := temporarySubscriptionIdleLifetime
	_, err := s.client.CreateSubscription(ctx, topic, subscription, &admin.CreateSubscriptionOptions{
		Properties: &admin.SubscriptionProperties{AutoDeleteOnIdle: &idleLifetime},
	})
	return err
}

func (s adminSubscriptions) deleteSubscription(ctx context.Context, topic, subscription string) error {
	_, err := s.client.DeleteSubscription(ctx, topic, subscription, nil)
	return err
}

func newTemporarySubscriptionName() string {
	id := make([]byte, temporarySubscriptionIDBytes)
	// crypto/rand.Read never returns an error.
	_, _ = rand.Read(id)
	return temporarySubscriptionPrefix + hex.EncodeToString(id)
}
