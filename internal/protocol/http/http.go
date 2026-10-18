package http

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptrace"
	"net/url"
	"strings"
	"time"

	"github.com/spedious/spedious/internal/check"
	"github.com/spedious/spedious/internal/extract"
	"github.com/spedious/spedious/internal/protocol"
	"github.com/spedious/spedious/internal/scenario"
)

// HTTPProtocol implements protocol.Protocol for HTTP/HTTPS requests.
type HTTPProtocol struct {
	Transport *http.Transport
}

// NewHTTPProtocol creates a new HTTP protocol with optimized connection pooling.
func NewHTTPProtocol() *HTTPProtocol {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        10000,
		MaxIdleConnsPerHost: 2000,
		MaxConnsPerHost:     0,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
		DisableCompression: false,
		ForceAttemptHTTP2:  true,
	}
	return &HTTPProtocol{Transport: transport}
}

func (p *HTTPProtocol) Name() string {
	return "http"
}

// NewVUClient creates an isolated HTTP client and cookie jar for a VU, sharing the transport pool.
func (p *HTTPProtocol) NewVUClient() (*http.Client, http.CookieJar) {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Transport: p.Transport,
		Jar:       jar,
		Timeout:   0, // per-request timeouts handled via context
	}
	return client, jar
}

// Execute performs an HTTP request step using the VU context.
func (p *HTTPProtocol) Execute(ctx context.Context, vuCtx *protocol.VUContext, step *scenario.RequestStep) (*protocol.ExecutionResult, error) {
	reqURL := vuCtx.Vars.EvaluateString(step.URL)
	reqMethod := strings.ToUpper(vuCtx.Vars.EvaluateString(step.Method))
	if reqMethod == "" {
		reqMethod = http.MethodGet
	}

	// Build query parameters
	if len(step.Query) > 0 {
		parsedURL, err := url.Parse(reqURL)
		if err != nil {
			return nil, fmt.Errorf("invalid URL %q: %w", reqURL, err)
		}
		q := parsedURL.Query()
		for k, v := range step.Query {
			q.Set(vuCtx.Vars.EvaluateString(k), vuCtx.Vars.EvaluateString(v))
		}
		parsedURL.RawQuery = q.Encode()
		reqURL = parsedURL.String()
	}

	// Prepare request body
	var rawBody []byte
	var isJSONBody bool
	var bytesSent int64
	if step.Body != nil {
		switch b := step.Body.(type) {
		case string:
			bodyStr := vuCtx.Vars.EvaluateString(b)
			rawBody = []byte(bodyStr)
			bytesSent = int64(len(rawBody))
		case map[string]any, []any:
			evalBody := vuCtx.Vars.EvaluateAny(b)
			bodyBytes, err := json.Marshal(evalBody)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal request body to JSON: %w", err)
			}
			rawBody = bodyBytes
			bytesSent = int64(len(rawBody))
			isJSONBody = true
		default:
			bodyStr := fmt.Sprintf("%v", b)
			rawBody = []byte(bodyStr)
			bytesSent = int64(len(rawBody))
		}
	}

	reqTimeout := 30 * time.Second
	if step.Timeout.Duration() > 0 {
		reqTimeout = step.Timeout.Duration()
	}

	// Use VU client or default
	client := vuCtx.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	var (
		resp           *http.Response
		timing         protocol.RequestTiming
		execStart      = time.Now()
		maxRetries     = 5
		lastErr        error
	)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		reqCtx, cancel := context.WithTimeout(ctx, reqTimeout)

		var (
			getConnStart   time.Time
			connectStart   time.Time
			tlsStart       time.Time
			writeStart     time.Time
			firstByteStart time.Time
		)

		trace := &httptrace.ClientTrace{
			GetConn: func(hostPort string) {
				getConnStart = time.Now()
			},
			GotConn: func(info httptrace.GotConnInfo) {
				if !getConnStart.IsZero() {
					timing.Blocked = time.Since(getConnStart)
				}
			},
			ConnectStart: func(network, addr string) {
				connectStart = time.Now()
			},
			ConnectDone: func(network, addr string, err error) {
				if !connectStart.IsZero() {
					timing.Connecting = time.Since(connectStart)
				}
			},
			TLSHandshakeStart: func() {
				tlsStart = time.Now()
			},
			TLSHandshakeDone: func(state tls.ConnectionState, err error) {
				if !tlsStart.IsZero() {
					timing.TLSHandshake = time.Since(tlsStart)
				}
			},
			WroteHeaders: func() {
				writeStart = time.Now()
			},
			WroteRequest: func(info httptrace.WroteRequestInfo) {
				if !writeStart.IsZero() {
					timing.Sending = time.Since(writeStart)
				}
				firstByteStart = time.Now()
			},
			GotFirstResponseByte: func() {
				if !firstByteStart.IsZero() {
					timing.Waiting = time.Since(firstByteStart)
				}
			},
		}

		var bodyReader io.Reader
		if len(rawBody) > 0 {
			bodyReader = bytes.NewReader(rawBody)
		}

		tracedCtx := httptrace.WithClientTrace(reqCtx, trace)
		req, err := http.NewRequestWithContext(tracedCtx, reqMethod, reqURL, bodyReader)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create HTTP request: %w", err)
		}

		// Set headers
		if isJSONBody && req.Header.Get("Content-Type") == "" {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, v := range step.Headers {
			req.Header.Set(vuCtx.Vars.EvaluateString(k), vuCtx.Vars.EvaluateString(v))
		}

		// Auth helpers
		if step.Auth != nil {
			switch strings.ToLower(step.Auth.Type) {
			case "bearer":
				token := vuCtx.Vars.EvaluateString(step.Auth.Token)
				req.Header.Set("Authorization", "Bearer "+token)
			case "basic":
				u := vuCtx.Vars.EvaluateString(step.Auth.Username)
				p := vuCtx.Vars.EvaluateString(step.Auth.Password)
				auth := base64.StdEncoding.EncodeToString([]byte(u + ":" + p))
				req.Header.Set("Authorization", "Basic "+auth)
			}
		}

		resp, err = client.Do(req)
		if err == nil {
			lastErr = nil
			defer cancel()
			break
		}
		cancel()
		lastErr = err

		if ctx.Err() != nil {
			break
		}
		if attempt < maxRetries && isTransientError(err) {
			time.Sleep(time.Duration(15*(attempt+1)) * time.Millisecond)
			continue
		}
		break
	}

	timing.TotalDuration = time.Since(execStart)

	res := &protocol.ExecutionResult{
		StepName:  step.Name,
		Method:    reqMethod,
		URL:       reqURL,
		Timing:    timing,
		BytesSent: bytesSent,
	}

	if lastErr != nil {
		res.Error = lastErr
		return res, lastErr
	}
	defer resp.Body.Close()

	res.StatusCode = resp.StatusCode
	res.Headers = resp.Header

	// Read body
	recvStart := time.Now()
	bodyBytes, err := io.ReadAll(resp.Body)
	timing.Receiving = time.Since(recvStart)
	res.Timing.Receiving = timing.Receiving
	res.Timing.TotalDuration = time.Since(execStart)

	if err != nil {
		res.Error = fmt.Errorf("error reading response body: %w", err)
		return res, res.Error
	}

	res.Body = bodyBytes
	res.BytesReceived = int64(len(bodyBytes))

	// Store special variables
	vuCtx.Vars.Set("status", resp.StatusCode)
	vuCtx.Vars.Set("latency", timing.TotalDuration.Milliseconds())

	// Run extractors if defined
	if len(step.Extract) > 0 {
		extracted, extErr := extract.Extract(step.Extract, extract.ResponseData{
			StatusCode: resp.StatusCode,
			Headers:    resp.Header,
			Body:       bodyBytes,
			Duration:   timing.TotalDuration,
		})
		if extErr != nil {
			res.Error = extErr
			return res, extErr
		}
		for k, v := range extracted {
			vuCtx.Vars.Set(k, v)
		}
	}

	// Run assertions if defined
	if len(step.Assert) > 0 {
		chkCtx := check.ResponseContext{
			StatusCode: resp.StatusCode,
			Headers:    resp.Header,
			Body:       bodyBytes,
			Duration:   timing.TotalDuration,
			Vars:       vuCtx.Vars,
		}
		allPassed, chkResults := check.EvaluateAll(step.Assert, chkCtx)
		if !allPassed {
			for _, cr := range chkResults {
				if !cr.Passed {
					res.AssertionFails++
					if res.Error == nil {
						res.Error = fmt.Errorf("assertion failed: %s (%s)", cr.Rule, cr.Message)
					}
				}
			}
		}
	}

	return res, nil
}

func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "refused") ||
		strings.Contains(s, "reset") ||
		strings.Contains(s, "broken pipe") ||
		strings.Contains(s, "no buffer space")
}
