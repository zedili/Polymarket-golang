package polymarket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// HTTPClient HTTP客户端
type HTTPClient struct {
	client     *http.Client
	baseURL    string
	retryCfg   RetryConfig
	reqTimeout time.Duration
}

// RetryConfig 控制 HTTPClient 的重试策略。
//
//   MaxAttempts:  总共最多尝试几次(>=1)。1 = 不重试。
//   BaseDelay:    第一次重试前等待。后续按指数 2x 退避。
//   MaxDelay:     退避上限,防止过长等待。
//   RetryStatuses: 哪些 HTTP 状态码触发重试(默认 429, 500, 502, 503, 504)。
//                  4xx 业务错误(400/401/403/404)不重试。
//
// **POST/PUT/DELETE 默认不重试**(可能产生重复订单/重复取消)。GET 可以安全重试。
// 如果你确认 POST 是幂等的(比如 PostOrderV2 的 idempotency 由服务端 nonce 保证),
// 可以把 RetryNonIdempotent 设为 true。
type RetryConfig struct {
	MaxAttempts        int
	BaseDelay          time.Duration
	MaxDelay           time.Duration
	RetryStatuses      []int
	RetryNonIdempotent bool
}

// DefaultRetryConfig 是 SDK 默认值:GET 最多 3 次,POST 默认不重试。
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:   3,
		BaseDelay:     200 * time.Millisecond,
		MaxDelay:      3 * time.Second,
		RetryStatuses: []int{429, 500, 502, 503, 504},
	}
}

// defaultHTTPTransport 给整个 SDK 共享一个 Transport,避免每个 HTTPClient
// 实例各自维护连接池(重复 TLS handshake)。
//
// 关键参数:
//   - MaxIdleConnsPerHost = 100(标准库默认是 2,对 Polymarket 单域名 API
//     在机器人场景下会反复建 TCP+TLS,延迟劣化严重)
//   - ForceAttemptHTTP2 = true(Polymarket 服务端支持 HTTP/2,多路复用)
//   - KeepAlive 30s(避免对端 RST 闲置连接)
var defaultHTTPTransport = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          200,
	MaxIdleConnsPerHost:   100,
	MaxConnsPerHost:       200,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

// bodyBufPool 复用 request body 的 bytes.Buffer,降低 GC 压力。
var bodyBufPool = sync.Pool{
	New: func() interface{} { return new(bytes.Buffer) },
}

// NewHTTPClient 创建新的HTTP客户端
func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		client: &http.Client{
			Transport: defaultHTTPTransport,
			Timeout:   30 * time.Second,
		},
		baseURL:    baseURL,
		retryCfg:   DefaultRetryConfig(),
		reqTimeout: 0, // 0 = 用 http.Client.Timeout
	}
}

// SetRetryConfig 自定义重试策略。
func (c *HTTPClient) SetRetryConfig(cfg RetryConfig) {
	if cfg.MaxAttempts < 1 {
		cfg.MaxAttempts = 1
	}
	c.retryCfg = cfg
}

// SetRequestTimeout 每次请求(含重试单次)最长等待。0 表示不另设 ctx 超时。
func (c *HTTPClient) SetRequestTimeout(d time.Duration) {
	c.reqTimeout = d
}

// Request 发送HTTP请求(自动重试,使用 background context)。
func (c *HTTPClient) Request(method, path string, headers map[string]string, body interface{}) (interface{}, error) {
	return c.RequestCtx(context.Background(), method, path, headers, body)
}

// RequestCtx 发送 HTTP 请求,可以被 ctx 取消;按 RetryConfig 重试 429/5xx。
//
// 重试规则:
//   - GET 默认重试
//   - POST/PUT/DELETE 默认不重试(防重复下单),设 RetryNonIdempotent=true 才重试
//   - 仅在 RetryStatuses 列表的 HTTP 状态码、io 错误、net 错误时重试
//   - 上下文取消(ctx.Done())时立即返回
func (c *HTTPClient) RequestCtx(ctx context.Context, method, path string, headers map[string]string, body interface{}) (interface{}, error) {
	url := c.baseURL + path

	// body 提前一次序列化为 []byte,这样重试时不必重新序列化。
	var bodyBytes []byte
	if body != nil {
		switch v := body.(type) {
		case string:
			bodyBytes = []byte(v)
		case []byte:
			bodyBytes = v
		default:
			b, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal body: %w", err)
			}
			bodyBytes = b
		}
	}

	idempotent := method == http.MethodGet || method == http.MethodHead || c.retryCfg.RetryNonIdempotent
	attempts := c.retryCfg.MaxAttempts
	if !idempotent || attempts < 1 {
		attempts = 1
	}

	var lastErr error
	delay := c.retryCfg.BaseDelay
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2
			if delay > c.retryCfg.MaxDelay && c.retryCfg.MaxDelay > 0 {
				delay = c.retryCfg.MaxDelay
			}
		}

		resp, retry, err := c.doOnce(ctx, method, url, headers, bodyBytes)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("after %d attempts: %w", attempts, lastErr)
}

// doOnce 发起单次 HTTP 调用。返回 (response, shouldRetry, error)。
func (c *HTTPClient) doOnce(ctx context.Context, method, url string, headers map[string]string, bodyBytes []byte) (interface{}, bool, error) {
	// 从 pool 拿一个 buffer 给本次 req body 用
	var reqBody io.Reader
	var buf *bytes.Buffer
	if len(bodyBytes) > 0 {
		buf = bodyBufPool.Get().(*bytes.Buffer)
		buf.Reset()
		buf.Write(bodyBytes)
		reqBody = buf
		defer bodyBufPool.Put(buf)
	}

	// 单次请求 timeout(可被外层 ctx 进一步收紧)
	if c.reqTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.reqTimeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "polymarket-sdk-go")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Content-Type", "application/json")

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		// 区分:ctx 取消不算可重试错误
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, false, err
		}
		// 网络层错误(connection refused / reset / tls error)默认可重试
		return nil, true, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// io.LimitReader 防意外/恶意巨大响应炸内存。Polymarket 单 endpoint 响应都在
	// MB 级,16 MiB 是宽松上限。命中即报错(而不是截断),避免 silent corruption。
	const maxRespBytes = 16 << 20
	lr := &io.LimitedReader{R: resp.Body, N: maxRespBytes + 1}
	respBody, err := io.ReadAll(lr)
	if err != nil {
		return nil, true, fmt.Errorf("failed to read response: %w", err)
	}
	if int64(len(respBody)) > maxRespBytes {
		return nil, false, fmt.Errorf("response body exceeded %d bytes", maxRespBytes)
	}

	if resp.StatusCode != http.StatusOK {
		retry := c.shouldRetryStatus(resp.StatusCode)
		return nil, retry, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// 服务端正常响应,**不**重试
	var jsonData interface{}
	if err := json.Unmarshal(respBody, &jsonData); err != nil {
		// 不是 JSON,返回原始字符串
		return string(respBody), false, nil
	}
	return jsonData, false, nil
}

func (c *HTTPClient) shouldRetryStatus(code int) bool {
	for _, s := range c.retryCfg.RetryStatuses {
		if s == code {
			return true
		}
	}
	return false
}

// Get 发送GET请求(自动重试)
func (c *HTTPClient) Get(path string, headers map[string]string) (interface{}, error) {
	return c.Request("GET", path, headers, nil)
}

// GetCtx GET with explicit context
func (c *HTTPClient) GetCtx(ctx context.Context, path string, headers map[string]string) (interface{}, error) {
	return c.RequestCtx(ctx, "GET", path, headers, nil)
}

// Post 发送POST请求(默认不重试,防重复)
func (c *HTTPClient) Post(path string, headers map[string]string, body interface{}) (interface{}, error) {
	return c.Request("POST", path, headers, body)
}

// PostCtx POST with explicit context
func (c *HTTPClient) PostCtx(ctx context.Context, path string, headers map[string]string, body interface{}) (interface{}, error) {
	return c.RequestCtx(ctx, "POST", path, headers, body)
}

// Delete 发送DELETE请求(默认不重试)
func (c *HTTPClient) Delete(path string, headers map[string]string, body interface{}) (interface{}, error) {
	return c.Request("DELETE", path, headers, body)
}

// DeleteCtx DELETE with explicit context
func (c *HTTPClient) DeleteCtx(ctx context.Context, path string, headers map[string]string, body interface{}) (interface{}, error) {
	return c.RequestCtx(ctx, "DELETE", path, headers, body)
}

// Put 发送PUT请求(默认不重试)
func (c *HTTPClient) Put(path string, headers map[string]string, body interface{}) (interface{}, error) {
	return c.Request("PUT", path, headers, body)
}

// PutCtx PUT with explicit context
func (c *HTTPClient) PutCtx(ctx context.Context, path string, headers map[string]string, body interface{}) (interface{}, error) {
	return c.RequestCtx(ctx, "PUT", path, headers, body)
}

// 静态检查避免空 import
var _ = strings.Contains
