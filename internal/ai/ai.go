// Package ai is a small provider-agnostic client that classifies chat messages
// as spam, scam, rule violations or normal conversation. [FEAT-020]
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Default models per provider, used when AI_MODEL is empty.
var defaultModels = map[string]string{
	"gemini": "gemini-3.1-flash-lite",
	"claude": "claude-haiku-5-5",
}

var defaultBaseURLs = map[string]string{
	"gemini": "https://generativelanguage.googleapis.com/v1beta",
	"openai": "https://api.openai.com/v1",
	"claude": "https://api.anthropic.com/v1",
}

// Verdict categories returned by Classify.
const (
	VerdictOK   = "ok"
	VerdictSpam = "spam"
	VerdictScam = "scam"
	VerdictRule = "rule"
)

// Keyword risk levels.
const (
	RiskLow    = "low"
	RiskMedium = "medium"
	RiskHigh   = "high"
)

// Keyword is a word or phrase the model suggests adding to the filter.
type Keyword struct {
	Text string `json:"text"`
	Risk string `json:"risk"`
}

// Verdict is the model's classification of one message.
type Verdict struct {
	Verdict    string    `json:"verdict"`
	Confidence float64   `json:"confidence"`
	Reason     string    `json:"reason"`
	Rule       string    `json:"rule"`
	Keywords   []Keyword `json:"keywords"`
}

// Client calls one configured AI provider.
type Client struct {
	provider string
	apiKey   string
	baseURL  string
	model    string
	http     *http.Client
}

// New returns a client, or nil if provider is empty (AI disabled).
func New(provider, apiKey, baseURL, model string) (*Client, error) {
	if provider == "" {
		return nil, nil
	}
	if _, ok := defaultBaseURLs[provider]; !ok {
		return nil, fmt.Errorf("unknown AI_PROVIDER %q (use gemini, openai or claude)", provider)
	}
	if model == "" {
		model = defaultModels[provider]
	}
	if model == "" {
		return nil, fmt.Errorf("AI_MODEL is required for provider %q", provider)
	}
	if baseURL == "" {
		baseURL = defaultBaseURLs[provider]
	}
	return &Client{
		provider: provider,
		apiKey:   apiKey,
		baseURL:  baseURL,
		model:    model,
		http:     &http.Client{Timeout: 65 * time.Second},
	}, nil
}

// Name describes the provider and model, for logs and /tgwatch_show settings.
func (c *Client) Name() string {
	return c.provider + "/" + c.model
}

// Classify sends the message with its context to the model and parses the verdict.
func (c *Client) Classify(ctx context.Context, req Request) (*Verdict, error) {
	system, user := buildPrompt(req)
	raw, err := c.complete(ctx, system, user)
	if err != nil {
		return nil, err
	}
	return parseVerdict(raw)
}

func (c *Client) complete(ctx context.Context, system, user string) (string, error) {
	switch c.provider {
	case "gemini":
		return c.completeGemini(ctx, system, user)
	case "openai":
		return c.completeOpenAI(ctx, system, user)
	case "claude":
		return c.completeClaude(ctx, system, user)
	}
	return "", fmt.Errorf("unknown provider %q", c.provider)
}

func (c *Client) completeGemini(ctx context.Context, system, user string) (string, error) {
	body := map[string]any{
		"systemInstruction": map[string]any{"parts": []map[string]string{{"text": system}}},
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]string{{"text": user}}},
		},
		"generationConfig": map[string]any{"responseMimeType": "application/json"},
	}
	var resp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	url := fmt.Sprintf("%s/models/%s:generateContent", c.baseURL, c.model)
	if err := c.post(ctx, url, map[string]string{"x-goog-api-key": c.apiKey}, body, &resp); err != nil {
		return "", err
	}
	if len(resp.Candidates) == 0 {
		return "", errors.New("gemini: empty response")
	}
	var sb strings.Builder
	for _, p := range resp.Candidates[0].Content.Parts {
		sb.WriteString(p.Text)
	}
	return sb.String(), nil
}

func (c *Client) completeOpenAI(ctx context.Context, system, user string) (string, error) {
	body := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"response_format": map[string]string{"type": "json_object"},
	}
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	headers := map[string]string{"Authorization": "Bearer " + c.apiKey}
	if err := c.post(ctx, c.baseURL+"/chat/completions", headers, body, &resp); err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("openai: empty response")
	}
	return resp.Choices[0].Message.Content, nil
}

func (c *Client) completeClaude(ctx context.Context, system, user string) (string, error) {
	body := map[string]any{
		"model":      c.model,
		"max_tokens": 1024,
		"system":     system,
		"messages":   []map[string]string{{"role": "user", "content": user}},
	}
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	headers := map[string]string{"x-api-key": c.apiKey, "anthropic-version": "2023-06-01"}
	if err := c.post(ctx, c.baseURL+"/messages", headers, body, &resp); err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, b := range resp.Content {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	if sb.Len() == 0 {
		return "", errors.New("claude: empty response")
	}
	return sb.String(), nil
}

// post sends a JSON request and decodes a JSON response. The API key is never logged.
func (c *Client) post(ctx context.Context, url string, headers map[string]string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s request: %w", c.provider, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		msg := string(data)
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return fmt.Errorf("%s: HTTP %d: %s", c.provider, resp.StatusCode, msg)
	}
	return json.Unmarshal(data, out)
}

// parseVerdict extracts the JSON object from the model output and validates it.
func parseVerdict(raw string) (*Verdict, error) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no JSON object in model output: %.200q", raw)
	}
	var v Verdict
	if err := json.Unmarshal([]byte(raw[start:end+1]), &v); err != nil {
		return nil, fmt.Errorf("bad JSON from model: %w", err)
	}
	v.Verdict = strings.ToLower(strings.TrimSpace(v.Verdict))
	switch v.Verdict {
	case VerdictOK, VerdictSpam, VerdictScam, VerdictRule:
	default:
		return nil, fmt.Errorf("unknown verdict %q", v.Verdict)
	}
	if v.Confidence < 0 {
		v.Confidence = 0
	}
	if v.Confidence > 1 {
		v.Confidence = 1
	}
	var kws []Keyword
	seen := map[string]bool{}
	for _, k := range v.Keywords {
		k.Text = strings.TrimSpace(k.Text)
		key := strings.ToLower(k.Text)
		if k.Text == "" || seen[key] {
			continue
		}
		seen[key] = true
		switch k.Risk = strings.ToLower(k.Risk); k.Risk {
		case RiskLow, RiskMedium, RiskHigh:
		default:
			k.Risk = RiskMedium
		}
		kws = append(kws, k)
		if len(kws) == MaxKeywords {
			break
		}
	}
	v.Keywords = kws
	return &v, nil
}
