package main

import (
	"context"
	"time"
)

// Capabilities describes what a provider can actually do, so the engine and UI
// can adapt without knowing which provider is in use. The Meta Cloud API, for
// example, cannot send free text to a business-initiated conversation — it
// requires a pre-approved template.
type Capabilities struct {
	FreeText         bool `json:"free_text"`
	Media            bool `json:"media"`
	RequiresTemplate bool `json:"requires_template"`
	ResolvesIDs      bool `json:"resolves_ids"`
}

// SenderInfo is one sending identity: a WhatsApp session for whatsmeow, a
// phone_number_id for Meta.
type SenderInfo struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Phone  string `json:"phone"`
	Status string `json:"status"` // connected | offline
}

// OutboundMessage is one rendered message ready to send.
type OutboundMessage struct {
	Text      string
	Media     []byte
	MediaMime string
	MediaName string
	Separate  bool // send media first, then Text as its own message

	// Meta Cloud API only; ignored by whatsmeow.
	TemplateName string
	TemplateLang string
	TemplateVars []string
}

type SendResult struct {
	ProviderMsgID string
	SentAt        time.Time
}

// SendError separates "try again later" from "this number will never work", so
// the worker does not burn retries on a permanently bad recipient.
type SendError struct {
	Err       error
	Retryable bool
	// FatalForSender means the sending identity itself is broken (session
	// dropped, token revoked) — the worker pauses the whole campaign rather
	// than failing every remaining recipient in quick succession.
	FatalForSender bool
}

func (e *SendError) Error() string { return e.Err.Error() }
func (e *SendError) Unwrap() error { return e.Err }

// Provider is the seam between the broadcast engine and however messages
// physically leave the building. The worker, schema, throttle, claim loop and
// retry logic are all provider-agnostic; swapping whatsmeow for the Meta Cloud
// API is an implementation of this interface plus a composer change in the UI.
type Provider interface {
	Name() string
	Capabilities() Capabilities

	// Senders lists usable sending identities.
	Senders(ctx context.Context) ([]SenderInfo, error)

	// Ready reports whether sender can send right now. Called before each
	// batch so a dropped session pauses the campaign instead of failing
	// every remaining recipient.
	Ready(ctx context.Context, sender string) error

	// Send delivers one message. Errors should be *SendError.
	Send(ctx context.Context, sender, phone string, msg OutboundMessage) (SendResult, error)

	// ResolveIDs maps opaque chat identifiers to phone numbers, returning the
	// resolved map and the identifiers that could not be resolved.
	ResolveIDs(ctx context.Context, sender string, ids []string) (map[string]string, []string, error)
}

// ── Dry-run wrapper ──────────────────────────────────────────────────────────

// dryRunProvider wraps a real provider and swallows the send. Everything else —
// audience resolution, pacing, claim/retry bookkeeping, cooldown and cap
// accounting — runs for real, so a dry run exercises the whole pipeline at zero
// risk to the WhatsApp account.
type dryRunProvider struct{ inner Provider }

func (d *dryRunProvider) Name() string                { return d.inner.Name() + "+dryrun" }
func (d *dryRunProvider) Capabilities() Capabilities  { return d.inner.Capabilities() }

func (d *dryRunProvider) Senders(ctx context.Context) ([]SenderInfo, error) {
	return d.inner.Senders(ctx)
}

func (d *dryRunProvider) Ready(ctx context.Context, sender string) error {
	return d.inner.Ready(ctx, sender)
}

func (d *dryRunProvider) Send(ctx context.Context, sender, phone string, msg OutboundMessage) (SendResult, error) {
	return SendResult{ProviderMsgID: "DRYRUN", SentAt: time.Now()}, nil
}

func (d *dryRunProvider) ResolveIDs(ctx context.Context, sender string, ids []string) (map[string]string, []string, error) {
	return d.inner.ResolveIDs(ctx, sender, ids)
}
