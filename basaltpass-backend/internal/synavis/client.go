// Package synavis provides a lightweight client for the Synavis Core OCaml billing engine.
//
// Phase 3 (Strimzi / Kafka integration):
// - After a Stripe payment succeeds, call NotifyFundsReceived to mirror the credit event to OCaml via Kafka.
// - All functions are fire-and-forget: if Kafka is unreachable, only a warning is logged.
// - Idempotency is enforced by the OCaml engine using stripe_payment_id as the dedup key.
package synavis

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

// Client is a minimal Kafka client for the Synavis Core engine.
type Client struct {
	brokers        string
	timeoutSeconds float64

	mu     sync.Mutex
	writer *kafka.Writer
}

// New creates a Client. brokers should be a comma-separated list of Kafka broker addresses.
func New(brokers string, timeoutSeconds float64) *Client {
	if brokers == "" {
		brokers = "localhost:9092"
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 5.0
	}
	return &Client{
		brokers:        brokers,
		timeoutSeconds: timeoutSeconds,
	}
}

// FundsReceivedPayload maps to the OCaml FundsReceived variant.
type FundsReceivedPayload struct {
	UserID          string `json:"user_id"`
	TenantID        string `json:"tenant_id"`
	Amount          int64  `json:"amount"`
	Currency        string `json:"currency"`   // e.g. "USD"
	StripePaymentID string `json:"stripe_payment_id"`
}

func (c *Client) getWriter() *kafka.Writer {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.writer == nil {
		brokerList := strings.Split(c.brokers, ",")
		c.writer = &kafka.Writer{
			Addr:                   kafka.TCP(brokerList...),
			Topic:                  "synavis-events",
			Balancer:               &kafka.LeastBytes{},
			BatchTimeout:           10 * time.Millisecond,
			WriteTimeout:           time.Duration(c.timeoutSeconds * float64(time.Second)),
			RequiredAcks:           kafka.RequireOne,
			AllowAutoTopicCreation: true,
		}
	}

	return c.writer
}

// publishEvent serialises the payload as a [variant, payload] JSON array and publishes it to Kafka.
func (c *Client) publishEvent(ctx context.Context, variant string, payload any) error {
	body, err := json.Marshal([]any{variant, payload})
	if err != nil {
		return fmt.Errorf("synavis: marshal error: %w", err)
	}

	w := c.getWriter()

	err = w.WriteMessages(ctx,
		kafka.Message{
			Value: body,
		},
	)
	if err != nil {
		return fmt.Errorf("synavis: publish failed: %w", err)
	}

	return nil
}

// NotifyFundsReceived mirrors a successful Stripe payment to the OCaml ledger via Kafka.
func (c *Client) NotifyFundsReceived(
	userID string,
	tenantID string,
	amountSmallest int64,
	stripePaymentID string,
	currency string,
) {
	if currency == "" {
		currency = "USD"
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.timeoutSeconds*float64(time.Second)))
	defer cancel()

	err := c.publishEvent(ctx, "FundsReceived", FundsReceivedPayload{
		UserID:          userID,
		TenantID:        tenantID,
		Amount:          amountSmallest,
		Currency:        currency,
		StripePaymentID: stripePaymentID,
	})
	if err != nil {
		log.Printf("[synavis] WARN: FundsReceived Kafka publish failed (user=%s stripe=%s): %v",
			userID, stripePaymentID, err)
	} else {
		log.Printf("[synavis] INFO: FundsReceived published to Kafka (user=%s stripe=%s amount=%d %s)",
			userID, stripePaymentID, amountSmallest, currency)
	}
}
