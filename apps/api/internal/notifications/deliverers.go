package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type DeliveryResult struct {
	ResponseCode int
	LatencyMs    int
	Skipped      bool
	Message      string
}

// Deliverer sends a notification through a channel type.
type Deliverer interface {
	Type() string
	Deliver(ctx context.Context, ch Channel, eventType string, payload map[string]any, credential string) (DeliveryResult, error)
}

type EmailDeliverer struct {
	Log *slog.Logger
}

func (EmailDeliverer) Type() string { return ChannelEmail }

func (d EmailDeliverer) Deliver(_ context.Context, ch Channel, eventType string, payload map[string]any, _ string) (DeliveryResult, error) {
	to, _ := ch.Config["to"].([]any)
	if len(to) == 0 {
		if s, ok := ch.Config["to"].(string); ok && strings.TrimSpace(s) != "" {
			to = []any{s}
		}
	}
	if len(to) == 0 {
		return DeliveryResult{}, fmt.Errorf("email channel requires config.to")
	}
	if d.Log != nil {
		d.Log.Info("notification email (dev sink)",
			slog.String("channelId", ch.ID.String()),
			slog.String("eventType", eventType),
			slog.Any("to", to),
			slog.Any("payloadKeys", keysOf(payload)),
		)
	}
	return DeliveryResult{ResponseCode: 202, LatencyMs: 0, Message: "accepted by email sink"}, nil
}

type WebhookDeliverer struct {
	Client *http.Client
}

func (WebhookDeliverer) Type() string { return ChannelWebhook }

func (d WebhookDeliverer) Deliver(ctx context.Context, ch Channel, eventType string, payload map[string]any, credential string) (DeliveryResult, error) {
	url, _ := ch.Config["url"].(string)
	url = strings.TrimSpace(url)
	if url == "" {
		return DeliveryResult{}, fmt.Errorf("webhook channel requires config.url")
	}
	body := map[string]any{
		"eventType": eventType,
		"payload":   sanitizePayload(payload),
		"channelId": ch.ID.String(),
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return DeliveryResult{}, err
	}
	client := d.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return DeliveryResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "DeployCore-Notifications/1.0")
	req.Header.Set("X-DeployCore-Event", eventType)
	if credential != "" {
		req.Header.Set("Authorization", "Bearer "+credential)
	}
	start := time.Now()
	res, err := client.Do(req)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		return DeliveryResult{LatencyMs: latency}, err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	if res.StatusCode >= 300 {
		return DeliveryResult{ResponseCode: res.StatusCode, LatencyMs: latency},
			fmt.Errorf("webhook returned status %d", res.StatusCode)
	}
	return DeliveryResult{ResponseCode: res.StatusCode, LatencyMs: latency}, nil
}

type ReservedDeliverer struct{ ChannelType string }

func (r ReservedDeliverer) Type() string { return r.ChannelType }

func (r ReservedDeliverer) Deliver(context.Context, Channel, string, map[string]any, string) (DeliveryResult, error) {
	return DeliveryResult{}, fmt.Errorf("channel type %s is reserved but not enabled yet", r.ChannelType)
}

func sanitizePayload(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "password") || strings.Contains(lk, "secret") ||
			strings.Contains(lk, "token") || strings.Contains(lk, "credential") {
			continue
		}
		out[k] = v
	}
	return out
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
