// Package fetch issues HTTP requests to configured endpoints.
package fetch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"usage-gauge/internal/types"
)

const (
	defaultTimeout = 10 * time.Second
	acceptLanguage = "en-US,en" // zai needs the international endpoint locale
)

// Response is the raw outcome of an endpoint request.
type Response struct {
	Status int
	Raw    string
	Body   map[string]any // nil when the body is not a JSON object
}

// Endpoint requests the endpoint and returns its raw response.
func Endpoint(ctx context.Context, ep types.EndpointConfig) (Response, error) {
	timeout := time.Duration(ep.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	method := ep.Methods
	if method == "" {
		method = "GET"
	}

	req, err := http.NewRequestWithContext(ctx, method, ep.URL, nil)
	if err != nil {
		return Response{}, err
	}
	// Configured headers (Authorization, Content-Type, ...), then the locale zai needs.
	for k, v := range ep.Headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("Accept-Language") == "" {
		req.Header.Set("Accept-Language", acceptLanguage)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{Status: resp.StatusCode}, err
	}

	// Best-effort JSON decode; parsers handle non-JSON via httpStatus + rawBody.
	var body map[string]any
	_ = json.Unmarshal(raw, &body)

	return Response{Status: resp.StatusCode, Raw: string(raw), Body: body}, nil
}
