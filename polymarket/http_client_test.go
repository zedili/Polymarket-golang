package polymarket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestRetryGET500 GET 遇 500 应该重试。
func TestRetryGET500(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&hits, 1)
		if n < 3 {
			http.Error(w, `"err"`, 500)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"ok": "yes"})
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL)
	c.SetRetryConfig(RetryConfig{MaxAttempts: 5, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond, RetryStatuses: []int{500}})
	resp, err := c.Get("/", nil)
	if err != nil {
		t.Fatalf("expected success after retries: %v", err)
	}
	if m, ok := resp.(map[string]interface{}); !ok || m["ok"] != "yes" {
		t.Errorf("unexpected resp: %v", resp)
	}
	if hits != 3 {
		t.Errorf("expected 3 calls (2 failed + 1 success), got %d", hits)
	}
}

// TestPostDoesNotRetryByDefault POST 默认不重试,避免重复下单。
func TestPostDoesNotRetryByDefault(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		http.Error(w, `"unavail"`, 503)
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL)
	c.SetRetryConfig(RetryConfig{MaxAttempts: 5, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond, RetryStatuses: []int{503}})
	_, err := c.Post("/order", nil, `{"x":1}`)
	if err == nil {
		t.Fatal("expected error")
	}
	if hits != 1 {
		t.Errorf("POST should not auto-retry, got %d calls", hits)
	}
}

// TestPostRetryWhenExplicitlyAllowed RetryNonIdempotent=true 时 POST 会重试。
func TestPostRetryWhenExplicitlyAllowed(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&hits, 1)
		if n < 2 {
			http.Error(w, `"unavail"`, 503)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"ok": "yes"})
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL)
	c.SetRetryConfig(RetryConfig{
		MaxAttempts:        3,
		BaseDelay:          time.Millisecond,
		MaxDelay:           5 * time.Millisecond,
		RetryStatuses:      []int{503},
		RetryNonIdempotent: true,
	})
	_, err := c.Post("/order", nil, `{"x":1}`)
	if err != nil {
		t.Fatalf("expected success on second attempt: %v", err)
	}
	if hits != 2 {
		t.Errorf("expected 2 attempts, got %d", hits)
	}
}

// TestContextCancelStopsRetry ctx 被取消时立即返回,不再重试。
func TestContextCancelStopsRetry(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		http.Error(w, `"unavail"`, 503)
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL)
	c.SetRetryConfig(RetryConfig{MaxAttempts: 10, BaseDelay: 50 * time.Millisecond, MaxDelay: 100 * time.Millisecond, RetryStatuses: []int{503}})

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	_, err := c.GetCtx(ctx, "/", nil)
	if err == nil {
		t.Fatal("expected ctx-cancelled error")
	}
	// 应该最多打 1 次(第一次同步发出),然后退避时被 ctx 取消
	if hits > 2 {
		t.Errorf("ctx cancel did not stop retry loop, hits=%d", hits)
	}
}

// TestNon200BusinessErrorNotRetried 4xx 业务错误不重试。
func TestNon200BusinessErrorNotRetried(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		http.Error(w, `"bad request"`, 400)
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL)
	c.SetRetryConfig(RetryConfig{MaxAttempts: 5, BaseDelay: time.Millisecond, RetryStatuses: []int{500, 503}})
	_, err := c.Get("/", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if hits != 1 {
		t.Errorf("400 should not retry, got %d", hits)
	}
}

// TestBodyBufferPoolReuse 验证 sync.Pool body buffer 不影响并发正确性。
// 跑 100 个并发 POST(允许重试),body 内容必须 byte-for-byte 一致。
func TestBodyBufferPoolReuse(t *testing.T) {
	expected := `{"hello":"world","big":"payload-that-is-not-short"}`
	var mismatched int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		got := string(buf[:n])
		if got != expected {
			atomic.AddInt64(&mismatched, 1)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL)
	c.SetRetryConfig(RetryConfig{MaxAttempts: 1, RetryStatuses: nil})

	const N = 100
	done := make(chan struct{}, N)
	for i := 0; i < N; i++ {
		go func() {
			_, _ = c.Post("/", nil, expected)
			done <- struct{}{}
		}()
	}
	for i := 0; i < N; i++ {
		<-done
	}
	if mismatched != 0 {
		t.Errorf("body buffer corruption: %d mismatched out of %d", mismatched, N)
	}
}
