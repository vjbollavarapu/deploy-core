package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type deliverResult struct {
	ResponseCode int
	LatencyMs    int
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

func signBody(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func randomSecret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func postSigned(ctx context.Context, client *http.Client, url string, eventType string, deliveryID uuidStringer, payload map[string]any, secret []byte) (deliverResult, error) {
	body := map[string]any{
		"id":        deliveryID.String(),
		"event":     eventType,
		"payload":   sanitizePayload(payload),
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return deliverResult{}, err
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return deliverResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "DeployCore-Webhooks/1.0")
	req.Header.Set("X-DeployCore-Event", eventType)
	req.Header.Set("X-DeployCore-Delivery", deliveryID.String())
	req.Header.Set("X-DeployCore-Signature", signBody(secret, raw))
	start := time.Now()
	res, err := client.Do(req)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		return deliverResult{LatencyMs: latency}, err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	if res.StatusCode >= 300 {
		return deliverResult{ResponseCode: res.StatusCode, LatencyMs: latency},
			fmt.Errorf("webhook returned status %d", res.StatusCode)
	}
	return deliverResult{ResponseCode: res.StatusCode, LatencyMs: latency}, nil
}

type uuidStringer interface {
	String() string
}
