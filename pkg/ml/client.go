package ml

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"stock-options/pkg/model"
)

type Client interface {
	PushAnalysisAndForecast(ctx context.Context, ticker string, forecast *model.ForecastResult, analysis *model.AdvancedAnalysis) (*model.ExternalMLInsight, error)
}

type HTTPClient struct {
	baseURL    string
	pushPath   string
	apiKey     string
	httpClient *http.Client
}

type pushRequest struct {
	Ticker    string                  `json:"ticker"`
	Forecast  *model.ForecastResult   `json:"forecast"`
	Analysis  *model.AdvancedAnalysis `json:"analysis"`
	Source    map[string]string       `json:"source"`
	Requested string                  `json:"requested_at"`
}

func NewHTTPClient(baseURL, pushPath, apiKey string, timeout time.Duration) *HTTPClient {
	baseURL = strings.TrimSpace(baseURL)
	pushPath = strings.TrimSpace(pushPath)
	if pushPath == "" {
		pushPath = "/ingest"
	}
	if !strings.HasPrefix(pushPath, "/") {
		pushPath = "/" + pushPath
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &HTTPClient{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		pushPath:   pushPath,
		apiKey:     strings.TrimSpace(apiKey),
		httpClient: &http.Client{Timeout: timeout},
	}
}

func NewHTTPClientFromEnv() *HTTPClient {
	baseURL := strings.TrimSpace(os.Getenv("ML_SERVICE_URL"))
	if baseURL == "" {
		return nil
	}
	pushPath := strings.TrimSpace(os.Getenv("ML_SERVICE_PUSH_PATH"))
	timeout := 10 * time.Second
	if raw := strings.TrimSpace(os.Getenv("ML_SERVICE_TIMEOUT_MS")); raw != "" {
		if ms, err := time.ParseDuration(raw + "ms"); err == nil {
			timeout = ms
		}
	}
	return NewHTTPClient(baseURL, pushPath, os.Getenv("ML_SERVICE_API_KEY"), timeout)
}

func (c *HTTPClient) PushAnalysisAndForecast(ctx context.Context, ticker string, forecast *model.ForecastResult, analysis *model.AdvancedAnalysis) (*model.ExternalMLInsight, error) {
	if c == nil || strings.TrimSpace(c.baseURL) == "" {
		return nil, fmt.Errorf("external ML service is not configured")
	}

	now := time.Now().UTC()
	reqBody := pushRequest{
		Ticker:   ticker,
		Forecast: forecast,
		Analysis: analysis,
		Source: map[string]string{
			"analysis_endpoint": fmt.Sprintf("/analysis?ticker=%s", ticker),
			"forecast_endpoint": fmt.Sprintf("/forecast?ticker=%s", ticker),
		},
		Requested: now.Format(time.RFC3339),
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+c.pushPath, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var raw map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		raw = map[string]interface{}{"decode_error": err.Error()}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("external ML service returned status %d", resp.StatusCode)
	}

	insight := parseExternalMLInsight(raw)
	insight.RequestedAt = now
	insight.ReceivedAt = time.Now().UTC()
	if insight.Status == "" {
		insight.Status = "ok"
	}
	if insight.Raw == nil {
		insight.Raw = raw
	}
	return insight, nil
}

func parseExternalMLInsight(raw map[string]interface{}) *model.ExternalMLInsight {
	insight := &model.ExternalMLInsight{
		Status: "ok",
		Raw:    raw,
	}
	if v, ok := raw["provider"].(string); ok {
		insight.Provider = v
	}
	if v, ok := raw["model"].(string); ok {
		insight.Model = v
	}
	if v, ok := raw["status"].(string); ok {
		insight.Status = v
	}
	if v, ok := raw["message"].(string); ok {
		insight.Message = v
	}

	rec, ok := raw["recommendation"].(map[string]interface{})
	if !ok {
		return insight
	}

	if v, ok := rec["action"].(string); ok {
		insight.Recommendation.Action = normalizeAction(v)
	}
	if v, ok := rec["confidence"].(string); ok {
		insight.Recommendation.Confidence = v
	}
	if v, ok := rec["score_delta"].(float64); ok {
		insight.Recommendation.ScoreDelta = int(v)
	}
	if v, ok := rec["rationale"].([]interface{}); ok {
		insight.Recommendation.Rationale = make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				insight.Recommendation.Rationale = append(insight.Recommendation.Rationale, s)
			}
		}
	}
	return insight
}

func normalizeAction(action string) string {
	a := strings.ToUpper(strings.TrimSpace(action))
	switch a {
	case "BUY", "HOLD", "SELL":
		return a
	default:
		return ""
	}
}
