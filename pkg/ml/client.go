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
	SubmitAnalysisAndForecast(ctx context.Context, ticker string, forecast *model.ForecastResult, analysis *model.AdvancedAnalysis) (*SubmitResult, error)
	GetJobStatus(ctx context.Context, jobID string) (*JobStatusResult, error)
}

type HTTPClient struct {
	baseURL    string
	pushPath   string
	statusPath string
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

type SubmitResult struct {
	JobID   string                   `json:"job_id,omitempty"`
	Status  string                   `json:"status"`
	Message string                   `json:"message,omitempty"`
	Insight *model.ExternalMLInsight `json:"insight,omitempty"`
}

type JobStatusResult struct {
	JobID   string                   `json:"job_id,omitempty"`
	Status  string                   `json:"status"`
	Message string                   `json:"message,omitempty"`
	Insight *model.ExternalMLInsight `json:"insight,omitempty"`
}

func NewHTTPClient(baseURL, pushPath, statusPath, apiKey string, timeout time.Duration) *HTTPClient {
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
	statusPath = strings.TrimSpace(statusPath)
	if statusPath == "" {
		statusPath = "/jobs/{job_id}"
	}
	if !strings.HasPrefix(statusPath, "/") {
		statusPath = "/" + statusPath
	}
	return &HTTPClient{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		pushPath:   pushPath,
		statusPath: statusPath,
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
	statusPath := strings.TrimSpace(os.Getenv("ML_SERVICE_STATUS_PATH"))
	timeout := 10 * time.Second
	if raw := strings.TrimSpace(os.Getenv("ML_SERVICE_TIMEOUT_MS")); raw != "" {
		if ms, err := time.ParseDuration(raw + "ms"); err == nil {
			timeout = ms
		}
	}
	return NewHTTPClient(baseURL, pushPath, statusPath, os.Getenv("ML_SERVICE_API_KEY"), timeout)
}

func (c *HTTPClient) SubmitAnalysisAndForecast(ctx context.Context, ticker string, forecast *model.ForecastResult, analysis *model.AdvancedAnalysis) (*SubmitResult, error) {
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

	if resp.StatusCode == http.StatusAccepted {
		jobID, _ := raw["job_id"].(string)
		status, _ := raw["status"].(string)
		message, _ := raw["message"].(string)
		if strings.TrimSpace(status) == "" {
			status = "queued"
		}
		return &SubmitResult{
			JobID:   strings.TrimSpace(jobID),
			Status:  strings.ToLower(strings.TrimSpace(status)),
			Message: strings.TrimSpace(message),
		}, nil
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
	return &SubmitResult{
		Status:  "completed",
		Message: "external ML analysis completed",
		Insight: insight,
	}, nil
}

func (c *HTTPClient) GetJobStatus(ctx context.Context, jobID string) (*JobStatusResult, error) {
	if c == nil || strings.TrimSpace(c.baseURL) == "" {
		return nil, fmt.Errorf("external ML service is not configured")
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}
	statusPath := strings.ReplaceAll(c.statusPath, "{job_id}", jobID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+statusPath, nil)
	if err != nil {
		return nil, err
	}
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
		return nil, fmt.Errorf("external ML status endpoint returned status %d", resp.StatusCode)
	}

	status, _ := raw["status"].(string)
	message, _ := raw["message"].(string)
	result := &JobStatusResult{
		JobID:   jobID,
		Status:  strings.ToLower(strings.TrimSpace(status)),
		Message: strings.TrimSpace(message),
	}
	if result.Status == "" {
		result.Status = "running"
	}

	if result.Status == "completed" || result.Status == "done" || result.Status == "ok" {
		insight := parseExternalMLInsight(raw)
		insight.RequestedAt = time.Now().UTC()
		insight.ReceivedAt = time.Now().UTC()
		result.Status = "completed"
		result.Insight = insight
	}
	return result, nil
}

func parseExternalMLInsight(raw map[string]interface{}) *model.ExternalMLInsight {
	if nested, ok := raw["result"].(map[string]interface{}); ok {
		return parseExternalMLInsight(nested)
	}

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
