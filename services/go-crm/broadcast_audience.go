package main

import (
	"context"
	"fmt"
	"sort"
)

// AudienceResult is what both the preview and the build produce. The same
// resolver serves both (persist on/off), so the number shown in the composer is
// the number that actually gets built.
type AudienceResult struct {
	Recipients      []*Recipient   `json:"-"`
	Total           int            `json:"total"`
	BySource        map[string]int `json:"by_source"`
	Skipped         map[string]int `json:"skipped"`
	UnresolvedCount int            `json:"unresolved_count"`
	Sample          []SampleRow    `json:"sample"`
	EstimateSeconds int            `json:"estimate_seconds"`
	Warnings        []string       `json:"warnings,omitempty"`
}

type SampleRow struct {
	Phone   string `json:"phone"`
	Name    string `json:"name"`
	Wilayah string `json:"wilayah"`
	Tier    string `json:"tier"`
	Source  string `json:"source"`
}

// Skip reasons.
const (
	SkipInvalid   = "invalid"
	SkipBotNumber = "bot_number"
	SkipBlacklist = "blacklist"
	SkipOptOut    = "optout"
	SkipCooldown  = "cooldown"
	SkipDup       = "dup"
)

// resolveAudience turns the campaign's audience spec into a concrete recipient
// list. Sources are merged in precedence order customers → upload → chat, first
// writer wins, because the customers table carries the curated name/tier/wilayah
// that personalization depends on.
func resolveAudience(ctx context.Context, b *Broadcast, prov Provider, thr Throttle) (*AudienceResult, error) {
	res := &AudienceResult{
		BySource: map[string]int{"customers": 0, "upload": 0, "chat": 0},
		Skipped:  map[string]int{},
		Sample:   []SampleRow{},
		Warnings: []string{},
	}

	// Keyed on normalized phone: this map is both the dedupe mechanism and the
	// precedence rule.
	byPhone := map[string]*Recipient{}
	order := []string{}

	add := func(r *Recipient) {
		if _, exists := byPhone[r.Phone]; exists {
			res.Skipped[SkipDup]++
			return
		}
		byPhone[r.Phone] = r
		order = append(order, r.Phone)
		res.BySource[r.Source]++
	}

	// ── Source (a): CRM customers ────────────────────────────────────────
	if b.Audience.Customers != nil {
		custs, err := dbCustomersForBroadcast(ctx,
			b.Audience.Customers.Tiers, b.Audience.Customers.Wilayahs)
		if err != nil {
			return nil, fmt.Errorf("ambil customer: %w", err)
		}
		for i := range custs {
			c := custs[i]
			phones := c.Phone
			if !b.AllPhones && len(phones) > 1 {
				// Extra numbers are almost always the same person's alternate
				// line; messaging both is a duplicate to them and a spam signal
				// to WhatsApp. Opt in via all_phones.
				phones = phones[:1]
			}
			for _, raw := range phones {
				phone := normPhone(raw)
				id := c.ID
				add(&Recipient{
					Phone: phone, Name: c.Name, Wilayah: c.Wilayah, Tier: c.Tier,
					CustomerID: &id, Source: "customers",
					Vars: map[string]string{
						"nama": c.Name, "wilayah": c.Wilayah, "tier": c.Tier,
					},
				})
			}
		}
	}

	// ── Source (b): uploaded file ────────────────────────────────────────
	for _, row := range b.Audience.Upload {
		phone := normPhone(row.Phone)
		vars := map[string]string{"nama": row.Name, "wilayah": row.Wilayah}
		for k, v := range row.Vars {
			vars[k] = v
		}
		add(&Recipient{
			Phone: phone, Name: row.Name, Wilayah: row.Wilayah,
			Source: "upload", Vars: vars,
		})
	}

	// ── Source (c): bot chat history (LID → phone) ───────────────────────
	if b.Audience.Chat != nil && len(b.Audience.Chat.ChatflowIDs) > 0 {
		chatIDs, err := dbChatIDsForChatflows(ctx, b.Audience.Chat.ChatflowIDs, b.Audience.Chat.Days)
		if err != nil {
			return nil, fmt.Errorf("ambil riwayat chat: %w", err)
		}
		sessions, err := dbWASessionsForChatflows(ctx, b.Audience.Chat.ChatflowIDs)
		if err != nil {
			return nil, fmt.Errorf("ambil sesi WA: %w", err)
		}

		wp, isWhatsmeow := prov.(*whatsmeowProvider)
		if !isWhatsmeow {
			if d, ok := prov.(*dryRunProvider); ok {
				wp, isWhatsmeow = d.inner.(*whatsmeowProvider)
			}
		}

		switch {
		case !isWhatsmeow:
			res.Warnings = append(res.Warnings,
				"provider ini tidak mendukung resolusi LID — sumber riwayat chat dilewati")
		case len(sessions) == 0:
			res.Warnings = append(res.Warnings,
				"tidak ada sesi WA aktif untuk chatflow terpilih — sumber riwayat chat dilewati")
		default:
			// The LID map is per-session SQLite, so an ID only resolves against
			// the session that saw it. Union across every session bound to the
			// selected chatflows; first session to resolve an ID wins.
			pending := chatIDs
			resolvedAll := map[string]string{}
			passthroughAll := map[string]string{}

			for _, sess := range sessions {
				if len(pending) == 0 {
					break
				}
				resolved, passthrough, unresolved, err := wp.ResolveIDsWithPassthrough(ctx, sess, pending)
				if err != nil {
					res.Warnings = append(res.Warnings,
						fmt.Sprintf("resolusi LID gagal di satu sesi: %v", err))
					continue
				}
				for k, v := range resolved {
					resolvedAll[k] = v
				}
				for k, v := range passthrough {
					passthroughAll[k] = v
				}
				pending = unresolved
			}

			// Anything still unresolved is reported, never silently dropped:
			// it is usually a contact from another platform sharing the
			// chatflow, or an old contact the session no longer knows.
			res.UnresolvedCount = len(pending)

			chatPhones := map[string]bool{}
			for _, phone := range resolvedAll {
				chatPhones[normPhone(phone)] = true
			}
			if b.Audience.Chat.IncludePassthrough {
				for _, phone := range passthroughAll {
					chatPhones[normPhone(phone)] = true
				}
			} else if len(passthroughAll) > 0 {
				res.Warnings = append(res.Warnings, fmt.Sprintf(
					"%d chatId sudah berbentuk nomor tapi tidak disertakan (bisa jadi dari platform lain)",
					len(passthroughAll)))
			}

			// Enrich with what we already know about these numbers.
			list := make([]string, 0, len(chatPhones))
			for p := range chatPhones {
				list = append(list, p)
			}
			sort.Strings(list)

			known, err := dbCustomerByPhoneSet(ctx, list)
			if err != nil {
				return nil, fmt.Errorf("enrich customer: %w", err)
			}
			for _, phone := range list {
				r := &Recipient{Phone: phone, Source: "chat", Vars: map[string]string{}}
				if c, ok := known[phone]; ok {
					id := c.ID
					r.Name, r.Wilayah, r.Tier, r.CustomerID = c.Name, c.Wilayah, c.Tier, &id
				}
				r.Vars["nama"] = r.Name
				r.Vars["wilayah"] = r.Wilayah
				r.Vars["tier"] = r.Tier
				add(r)
			}
		}
	}

	// ── Exclusion pipeline ───────────────────────────────────────────────
	// Applied here at build time and again at send time, because a campaign
	// scheduled for next week must honour an opt-out recorded tomorrow.
	phones := make([]string, 0, len(order))
	for _, p := range order {
		phones = append(phones, p)
	}

	botPhones := map[string]bool{}
	if senders, err := prov.Senders(ctx); err == nil {
		for _, s := range senders {
			if s.Phone != "" {
				botPhones[normPhone(s.Phone)] = true
			}
		}
	}

	blacklisted, err := dbBlacklistedPhoneSet(ctx, phones)
	if err != nil {
		return nil, fmt.Errorf("cek blacklist: %w", err)
	}
	optedOut, err := dbOptedOutSet(ctx, phones)
	if err != nil {
		return nil, fmt.Errorf("cek opt-out: %w", err)
	}
	recentlyContacted, err := dbRecentlyContactedSet(ctx, phones, thr.CooldownDays)
	if err != nil {
		return nil, fmt.Errorf("cek cooldown: %w", err)
	}

	for _, phone := range order {
		r := byPhone[phone]
		switch {
		case !validWAPhone(phone):
			r.Status, r.SkipReason = RStatusSkipped, SkipInvalid
		case botPhones[phone]:
			// Our own bot numbers reach the audience through their LIDs.
			r.Status, r.SkipReason = RStatusSkipped, SkipBotNumber
		case (r.Tier == "blacklist" || blacklisted[phone]) && !b.IncludeBlacklist:
			// Tier is known directly for customer-sourced rows; the phone
			// lookup catches upload/chat rows that happen to be blacklisted.
			r.Status, r.SkipReason = RStatusSkipped, SkipBlacklist
		case optedOut[phone]:
			r.Status, r.SkipReason = RStatusSkipped, SkipOptOut
		case recentlyContacted[phone]:
			r.Status, r.SkipReason = RStatusSkipped, SkipCooldown
		default:
			r.Status = RStatusPending
		}
		if r.SkipReason != "" {
			res.Skipped[r.SkipReason]++
		} else {
			res.Total++
			if len(res.Sample) < 10 {
				res.Sample = append(res.Sample, SampleRow{
					Phone: r.Phone, Name: r.Name, Wilayah: r.Wilayah,
					Tier: r.Tier, Source: r.Source,
				})
			}
		}
		res.Recipients = append(res.Recipients, r)
	}

	res.EstimateSeconds = thr.estimateSeconds(res.Total)

	if res.Total == 0 && len(res.Recipients) > 0 {
		res.Warnings = append(res.Warnings,
			"semua kandidat penerima tersaring — cek rincian 'dilewati' di bawah")
	}
	if res.Total > maxRecipients {
		return nil, fmt.Errorf("penerima %d melebihi batas %d", res.Total, maxRecipients)
	}
	return res, nil
}
