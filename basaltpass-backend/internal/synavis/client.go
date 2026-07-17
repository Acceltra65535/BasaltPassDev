// Package synavis provides a lightweight HTTP client for the Synavis Core OCaml billing engine.
//
// Phase 1 (in-memory + HTTP integration):
// - After a Stripe payment succeeds, call NotifyFundsReceived to mirror the credit event to OCaml.
// - All functions are fire-and-forget: if the engine is unreachable, only a warning is logged.
// - Idempotency is enforced by the OCaml engine using stripe_payment_id as the dedup key.
package synavis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// Client is a minimal HTTP client for the Synavis Core engine.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New creates a Client. baseURL should be the engine base URL (e.g. "http://localhost:10622").
// timeoutSeconds controls how long to wait before giving up.
func New(baseURL string, timeoutSeconds float64) *Client {
	if baseURL == "" {
		baseURL = "http://localhost:10622"
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 5.0
	}
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutSeconds * float64(time.Second)),
		},
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

// postEvent serialises the payload as a [variant, payload] JSON array and POSTs it.
func (c *Client) postEvent(ctx context.Context, variant string, payload any) error {
	body, err := json.Marshal([]any{variant, payload})
	if err != nil {
		return fmt.Errorf("synavis: marshal error: %w", err)
	}

	url := c.baseURL + "/api/b1/events"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("synavis: build request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("synavis: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}
	return fmt.Errorf("synavis: engine returned status %d", resp.StatusCode)
}

// NotifyFundsReceived mirrors a successful Stripe payment to the OCaml ledger.
// This is fire-and-forget: the error is only logged, never returned to the caller.
//
// Parameters:
//   - userID:          BasaltPass user ID (string form)
//   - tenantID:        Tenant ID associated with the payment
//   - amountSmallest:  Amount in the smallest currency unit (e.g. cents × 1_000_000 for microcredits)
//   - stripePaymentID: Stripe payment_intent ID, used for idempotency by the OCaml engine
//   - currency:        ISO-4217 code, e.g. "USD"
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

	ctx, cancel := context.WithTimeout(context.Background(), c.httpClient.Timeout)
	defer cancel()

	err := c.postEvent(ctx, "FundsReceived", FundsReceivedPayload{
		UserID:          userID,
		TenantID:        tenantID,
		Amount:          amountSmallest,
		Currency:        currency,
		StripePaymentID: stripePaymentID,
	})
	if err != nil {
		// Only warn – the OCaml mirror must never break BasaltPass payment flows.
		log.Printf("[synavis] WARN: FundsReceived event mirror failed (user=%s stripe=%s): %v",
			userID, stripePaymentID, err)
	} else {
		log.Printf("[synavis] INFO: FundsReceived mirrored (user=%s stripe=%s amount=%d %s)",
			userID, stripePaymentID, amountSmallest, currency)
	}
}
