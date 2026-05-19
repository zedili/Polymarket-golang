package polymarket

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testPK 和 testAddr 与 order_builder 包的常量保持一致(私钥 = 0x11..11)。
const (
	v2TestPK   = "0x" + "11" + "11111111111111111111111111111111111111111111111111111111111111"
	v2TestAddr = "0x19E7E376E7C213B7E7e7e46cc70A5dD086DAff2A"
)

// 一个最小化的 mock CLOB 服务器:按路径返回固定 JSON。
// 注意:本 mock 不持有任何可变状态,所以多个 HTTP goroutine 并发调用是安全的。
// 调用方需要计数的话,自己用 atomic 计数器(versionFn / postOrder 闭包里就行)。
type mockHandler struct {
	versionFn func() int
	tickSize  string
	negRisk   bool
	postOrder func(body []byte) interface{}
}

func (m *mockHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == Version:
		v := 2
		if m.versionFn != nil {
			v = m.versionFn()
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"version": v})
	case r.URL.Path == GetTickSize:
		ts := "0.01"
		if m.tickSize != "" {
			ts = m.tickSize
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"minimum_tick_size": ts})
	case r.URL.Path == GetNegRisk:
		json.NewEncoder(w).Encode(map[string]interface{}{"neg_risk": m.negRisk})
	case r.URL.Path == GetFeeRate:
		json.NewEncoder(w).Encode(map[string]interface{}{"base_fee": 0})
	case r.URL.Path == GetOrderBook:
		// 简单的 orderbook,bids=[0.50@200], asks=[0.55@200]
		json.NewEncoder(w).Encode(map[string]interface{}{
			"market":    "m",
			"asset_id":  r.URL.Query().Get("token_id"),
			"timestamp": "0",
			"asks":      []map[string]interface{}{{"price": "0.55", "size": "200"}},
			"bids":      []map[string]interface{}{{"price": "0.50", "size": "200"}},
		})
	case r.URL.Path == PostOrder:
		body, _ := io.ReadAll(r.Body)
		var resp interface{} = map[string]interface{}{"success": true}
		if m.postOrder != nil {
			resp = m.postOrder(body)
		}
		json.NewEncoder(w).Encode(resp)
	case strings.HasPrefix(r.URL.Path, GetMarketByToken):
		tok := strings.TrimPrefix(r.URL.Path, GetMarketByToken)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"condition_id": "cond-for-" + tok,
		})
	case strings.HasPrefix(r.URL.Path, GetClobMarket):
		cid := strings.TrimPrefix(r.URL.Path, GetClobMarket)
		// 把 condition id 反解回原始 token id(mock 约定:cond-for-<token>),
		// 否则 feeInfos 缓存的 key 与调用者使用的 tokenID 不一致。
		realToken := strings.TrimPrefix(cid, "cond-for-")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"c":   cid,
			"mts": "0.01",
			"nr":  false,
			"t": []map[string]interface{}{
				{"t": realToken, "o": "YES"},
				{"t": realToken + "-no", "o": "NO"},
			},
			"fd": map[string]interface{}{"r": 0.02, "e": 1},
		})
	case strings.HasPrefix(r.URL.Path, GetBuilderFeeRate):
		json.NewEncoder(w).Encode(map[string]interface{}{
			"builder_maker_fee_rate_bps": 50,
			"builder_taker_fee_rate_bps": 100,
		})
	default:
		http.Error(w, "unmocked: "+r.URL.Path, http.StatusNotFound)
	}
}

func newMockClient(t *testing.T, h *mockHandler) (*ClobClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	creds := &ApiCreds{APIKey: "k", APISecret: testSecret, APIPassphrase: "p"}
	client, err := NewClobClient(srv.URL, 137, v2TestPK, creds, nil, "")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client, srv
}

func TestGetVersionReturnsServerVersion(t *testing.T) {
	c, _ := newMockClient(t, &mockHandler{versionFn: func() int { return 2 }})
	if v := c.GetVersion(); v != 2 {
		t.Errorf("version = %d, want 2", v)
	}
}

func TestGetVersionDefaultsToV2OnFailure(t *testing.T) {
	// 不挂任何 handler,直接连一个已关闭的 server。
	creds := &ApiCreds{APIKey: "k", APISecret: testSecret, APIPassphrase: "p"}
	client, err := NewClobClient("http://127.0.0.1:1", 137, v2TestPK, creds, nil, "")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if v := client.GetVersion(); v != 2 {
		t.Errorf("expected default 2 on network failure, got %d", v)
	}
}

func TestResolveOrderVersionCaches(t *testing.T) {
	var calls int64
	h := &mockHandler{versionFn: func() int { atomic.AddInt64(&calls, 1); return 2 }}
	c, _ := newMockClient(t, h)
	v1 := c.ResolveOrderVersion(false)
	v2 := c.ResolveOrderVersion(false)
	if v1 != 2 || v2 != 2 {
		t.Fatalf("expected 2, got %d/%d", v1, v2)
	}
	if got := atomic.LoadInt64(&calls); got != 1 {
		t.Errorf("expected /version to be hit once due to caching, got %d", got)
	}
	c.ResolveOrderVersion(true)
	if got := atomic.LoadInt64(&calls); got != 2 {
		t.Errorf("force refresh should hit /version, total calls = %d", got)
	}
}

func TestSetCachedOrderVersionAvoidsNetwork(t *testing.T) {
	var calls int64
	h := &mockHandler{versionFn: func() int { atomic.AddInt64(&calls, 1); return 2 }}
	c, _ := newMockClient(t, h)
	c.SetCachedOrderVersion(2)
	if v := c.ResolveOrderVersion(false); v != 2 {
		t.Errorf("expected cached version 2, got %d", v)
	}
	if got := atomic.LoadInt64(&calls); got != 0 {
		t.Errorf("expected no /version call, got %d", got)
	}
}

func TestIsOrderVersionMismatch(t *testing.T) {
	cases := []struct {
		resp interface{}
		want bool
	}{
		{nil, false},
		{map[string]interface{}{"success": true}, false},
		{map[string]interface{}{"error": "something else"}, false},
		{map[string]interface{}{"error": "order_version_mismatch: expected 2"}, true},
		{map[string]interface{}{"error": map[string]interface{}{"code": "order_version_mismatch"}}, true},
	}
	for i, c := range cases {
		got := IsOrderVersionMismatch(c.resp)
		if got != c.want {
			t.Errorf("case %d: got %v want %v (input=%v)", i, got, c.want, c.resp)
		}
	}
}

// retryOnVersionUpdate 只在 fn 自己刷新了缓存(典型场景是 PostOrderV2 检测到
// order_version_mismatch)才会重试 —— 否则不应再 POST。
func TestRetryOnVersionUpdateRetriesOnlyWhenFnInvalidatesCache(t *testing.T) {
	h := &mockHandler{versionFn: func() int { return 2 }}
	c, _ := newMockClient(t, h)
	c.SetCachedOrderVersion(1)

	var attempts int
	_, err := c.retryOnVersionUpdate(func() (interface{}, error) {
		attempts++
		// 第一次模拟 PostOrderV2 内部 force-refresh,使缓存变成 2
		if attempts == 1 {
			c.ResolveOrderVersion(true)
		}
		return attempts, nil
	})
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected exactly 2 attempts after fn invalidated cache, got %d", attempts)
	}
}

// 如果 fn 没有刷新缓存(即没真的发生 version mismatch),即使有别的进程
// 改了服务器版本,本地也不会自动重发 —— 关键安全性质:不会重复下单。
func TestRetryOnVersionUpdateNoRetryWhenFnDoesNotInvalidate(t *testing.T) {
	h := &mockHandler{versionFn: func() int { return 2 }}
	c, _ := newMockClient(t, h)
	c.SetCachedOrderVersion(2)

	var attempts int
	_, err := c.retryOnVersionUpdate(func() (interface{}, error) {
		attempts++
		return nil, nil
	})
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt when fn doesn't invalidate cache, got %d", attempts)
	}
}

func TestGetMarketByTokenCachesConditionID(t *testing.T) {
	c, _ := newMockClient(t, &mockHandler{})
	m, err := c.GetMarketByToken("tokA")
	if err != nil {
		t.Fatalf("market by token: %v", err)
	}
	if m["condition_id"] != "cond-for-tokA" {
		t.Errorf("unexpected response: %v", m)
	}
	c.mu.RLock()
	cid := c.tokenCondition["tokA"]
	c.mu.RUnlock()
	if cid != "cond-for-tokA" {
		t.Errorf("token→condition not cached: %s", cid)
	}
}

func TestGetClobMarketInfoPopulatesCaches(t *testing.T) {
	c, _ := newMockClient(t, &mockHandler{})
	md, err := c.GetClobMarketInfo("cond-X")
	if err != nil {
		t.Fatalf("clob market info: %v", err)
	}
	if md.MinTickSize != "0.01" {
		t.Errorf("tick size = %s", md.MinTickSize)
	}
	// mock 的 GetClobMarket 把 conditionID 反解到 token id(用于让 fee/tick 缓存
	// 能命中调用者使用的 token id)。对于 "cond-X"(无 cond-for- 前缀),
	// mock 会回填 "cond-X" 自身和 "cond-X-no" 两个 token。
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.tickSizes["cond-X"] != TickSize("0.01") {
		t.Error("tick size cache not populated")
	}
	if c.feeInfos["cond-X"] == nil {
		t.Error("fee info not populated")
	}
	if c.tokenCondition["cond-X"] != "cond-X" {
		t.Error("token-condition mapping not populated")
	}
}

func TestGetBuilderFeeRateConverts(t *testing.T) {
	c, _ := newMockClient(t, &mockHandler{})
	rate, err := c.GetBuilderFeeRate("0xdead")
	if err != nil {
		t.Fatalf("builder fee rate: %v", err)
	}
	if rate.Maker != 0.005 || rate.Taker != 0.01 {
		t.Errorf("rate conversion off: maker=%v taker=%v", rate.Maker, rate.Taker)
	}
	// Calling again should hit cache (no easy way to assert without counter; but at least no error).
	rate2, _ := c.GetBuilderFeeRate("0xdead")
	if rate != rate2 {
		t.Error("expected cached pointer to be returned")
	}
}

func TestGetBuilderFeeRateZeroCodeIsNoop(t *testing.T) {
	c, _ := newMockClient(t, &mockHandler{})
	rate, err := c.GetBuilderFeeRate(Bytes32Zero)
	if err != nil {
		t.Fatalf("zero code: %v", err)
	}
	if rate.Maker != 0 || rate.Taker != 0 {
		t.Errorf("zero code should return zero rate, got %+v", rate)
	}
}

func TestCreateOrderV2EndToEnd(t *testing.T) {
	c, _ := newMockClient(t, &mockHandler{})
	c.SetCachedOrderVersion(2)

	order, err := c.CreateOrderV2(&OrderArgsV2{
		TokenID:    "12345",
		Price:      0.5,
		Size:       10,
		Side:       BUY,
		Expiration: 0,
	}, nil)
	if err != nil {
		t.Fatalf("create order v2: %v", err)
	}
	if order.Signature == "" {
		t.Error("order signature is empty")
	}
	if order.Maker.Hex() == "" {
		t.Error("maker is empty")
	}
	if order.Timestamp == nil || order.Timestamp.Sign() <= 0 {
		t.Error("timestamp should be auto-populated")
	}
	// JSON payload roundtrip
	payload := order.ToJSONPayload("api-key", string(OrderTypeGTC), false, false)
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var roundtrip map[string]interface{}
	if err := json.Unmarshal(b, &roundtrip); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if roundtrip["orderType"] != "GTC" {
		t.Errorf("orderType missing in payload")
	}
}

func TestCreateAndPostOrderV2HappyPath(t *testing.T) {
	var postSeen []byte
	h := &mockHandler{
		versionFn: func() int { return 2 },
		postOrder: func(body []byte) interface{} {
			postSeen = body
			return map[string]interface{}{"orderID": "order-1", "success": true}
		},
	}
	c, _ := newMockClient(t, h)

	res, err := c.CreateAndPostOrderV2(&OrderArgsV2{
		TokenID:    "12345",
		Price:      0.5,
		Size:       10,
		Side:       BUY,
		Expiration: 0,
	}, nil, OrderTypeGTC, false, false)
	if err != nil {
		t.Fatalf("create+post: %v", err)
	}
	if r, ok := res.Response.(map[string]interface{}); !ok || r["success"] != true {
		t.Errorf("unexpected response: %v", res.Response)
	}
	if len(postSeen) == 0 {
		t.Fatal("POST /order body not seen")
	}
	if !strings.Contains(string(postSeen), `"signatureType":0`) {
		t.Errorf("expected signatureType=0 in body, got %s", postSeen)
	}
	if !strings.Contains(string(postSeen), `"timestamp":"`) {
		t.Errorf("expected timestamp field in V2 payload, got %s", postSeen)
	}
	if !strings.Contains(string(postSeen), `"metadata":"0x0000`) {
		t.Errorf("expected default metadata bytes32 zero, got %s", postSeen)
	}
}

func TestCreateOrderV2BuilderConfigDefaults(t *testing.T) {
	c, _ := newMockClient(t, &mockHandler{})
	c.SetCachedOrderVersion(2)
	c.SetBuilderConfig(&BuilderConfig{
		BuilderCode: "0xabc0000000000000000000000000000000000000000000000000000000000001",
	})
	order, err := c.CreateOrderV2(&OrderArgsV2{
		TokenID:    "12345",
		Price:      0.5,
		Size:       10,
		Side:       BUY,
		Expiration: 0,
	}, nil)
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	got := fmt.Sprintf("%x", order.Builder[:])
	if got != "abc0000000000000000000000000000000000000000000000000000000000001" {
		t.Errorf("builder code should come from BuilderConfig, got %s", got)
	}
}

// TestPostOrderV2VersionMismatchInvalidatesCache 模拟 Python 行为:
// 服务器返回 order_version_mismatch 错误时,SDK 应该强制刷新版本缓存。
func TestPostOrderV2VersionMismatchInvalidatesCache(t *testing.T) {
	var versionHit int64
	h := &mockHandler{
		versionFn: func() int {
			atomic.AddInt64(&versionHit, 1)
			return 2 // 服务器已经是 V2
		},
		postOrder: func(body []byte) interface{} {
			return map[string]interface{}{"error": "order_version_mismatch: server now on v2"}
		},
	}
	c, _ := newMockClient(t, h)
	c.SetCachedOrderVersion(1) // 假装一开始我们认为是 V1
	// 还没调过 GetVersion
	if got := atomic.LoadInt64(&versionHit); got != 0 {
		t.Fatalf("setup leaked /version call: %d", got)
	}

	order, err := c.CreateOrderV2(&OrderArgsV2{
		TokenID: "12345",
		Price:   0.5,
		Size:    1,
		Side:    BUY,
	}, nil)
	if err != nil {
		t.Fatalf("create v2 order: %v", err)
	}
	if _, err := c.PostOrderV2(order, OrderTypeGTC, false, false); err != nil {
		t.Fatalf("post: %v", err)
	}

	// PostOrderV2 见到 version mismatch 应主动调过一次 /version
	if got := atomic.LoadInt64(&versionHit); got < 1 {
		t.Errorf("expected /version refresh after mismatch, hits=%d", got)
	}
	// cache 现在应为新的服务端版本 (2)
	if v := c.ResolveOrderVersion(false); v != 2 {
		t.Errorf("expected cache refreshed to 2 after mismatch, got %d", v)
	}
}

func TestPostOrderV2RejectsPostOnlyFOK(t *testing.T) {
	c, _ := newMockClient(t, &mockHandler{})
	c.SetCachedOrderVersion(2)
	order, err := c.CreateOrderV2(&OrderArgsV2{
		TokenID: "12345",
		Price:   0.5,
		Size:    10,
		Side:    BUY,
	}, nil)
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	_, err = c.PostOrderV2(order, OrderTypeFOK, true, false)
	if err == nil {
		t.Error("expected error for postOnly + FOK")
	}
}

// ============================================================
//  B 类:V2 真实路径补充覆盖
// ============================================================

// TestCreateMarketOrderV2EndToEnd — 客户端层端到端 V2 市价单。
// 价格 0 时会调 CalculateMarketPrice -> GetOrderBook(asks 顶价 0.55)。
func TestCreateMarketOrderV2EndToEnd(t *testing.T) {
	c, _ := newMockClient(t, &mockHandler{})
	c.SetCachedOrderVersion(2)

	order, err := c.CreateMarketOrderV2(&MarketOrderArgsV2{
		TokenID:   "12345",
		Amount:    100, // 100 USDC 买入
		Side:      BUY,
		OrderType: OrderTypeFOK,
		// Price=0,触发 orderbook 计算
	}, nil)
	if err != nil {
		t.Fatalf("market v2: %v", err)
	}
	if order.Signature == "" {
		t.Error("missing signature")
	}
	// V2 市价单 expiration 必须固定 "0"(Python: ts="0" 即 maker order 永不过期)
	if order.Expiration.Sign() != 0 {
		t.Errorf("market order expiration should be 0, got %s", order.Expiration.String())
	}
	if order.Timestamp == nil || order.Timestamp.Sign() <= 0 {
		t.Error("timestamp must auto-populate")
	}
	// V2 round_down: 0.55 已经在 tick 上,maker=100 USDC,taker=100/0.55≈181.818181
	// makerAmount 单位是 1e6: 100_000_000;taker ≈ 181_818_100
	if order.MakerAmount.String() != "100000000" {
		t.Errorf("makerAmount=%s, want 100000000", order.MakerAmount.String())
	}
	if order.TakerAmount.String() != "181818100" {
		t.Errorf("takerAmount=%s, want 181818100 (V2 round_down at 0.55)", order.TakerAmount.String())
	}
}

// TestBuilderConfigConcurrentSetAndRead —— 验证 SetBuilderConfig +
// CreateOrderV2 并发跑 -race 干净(机器人热更 builder code + 下单的真实场景)。
func TestBuilderConfigConcurrentSetAndRead(t *testing.T) {
	c, _ := newMockClient(t, &mockHandler{})
	c.SetCachedOrderVersion(2)

	codes := []string{
		"0x" + strings.Repeat("11", 32),
		"0x" + strings.Repeat("22", 32),
		"0x" + strings.Repeat("33", 32),
		"",
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// writer:反复 set 不同 builder code,模拟热更新
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					code := codes[i%len(codes)]
					c.SetBuilderConfig(&BuilderConfig{BuilderCode: code})
					i++
				}
			}
		}(i)
	}

	// reader:并发下 V2 单
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_, _ = c.CreateOrderV2(&OrderArgsV2{
					TokenID: "12345",
					Price:   0.5,
					Size:    10,
					Side:    BUY,
				}, nil)
			}
		}()
	}
	// 让 reader 跑完后停 writer
	time.Sleep(20 * time.Millisecond)
	close(stop)
	wg.Wait()
}

// TestBuilderConfigCopyOnSetAndGet —— 外部 mutate 不应该影响内部状态。
func TestBuilderConfigCopyOnSetAndGet(t *testing.T) {
	c, _ := newMockClient(t, &mockHandler{})
	cfg := &BuilderConfig{BuilderCode: "0xaaa"}
	c.SetBuilderConfig(cfg)
	// 外部 mutate 入参 —— SDK 内部应已 copy,不受影响
	cfg.BuilderCode = "0xbbb"
	got := c.GetBuilderConfig()
	if got == nil || got.BuilderCode != "0xaaa" {
		t.Errorf("internal state was mutated via input pointer: got %+v", got)
	}
	// 外部 mutate getter 返回值 —— 同理不应影响下一次 get
	got.BuilderCode = "0xccc"
	again := c.GetBuilderConfig()
	if again == nil || again.BuilderCode != "0xaaa" {
		t.Errorf("internal state was mutated via getter return: got %+v", again)
	}
}

// TestPostOrderV2RejectsNilOrder —— 公共 API 收到 nil 必须 error 不能 panic。
func TestPostOrderV2RejectsNilOrder(t *testing.T) {
	c, _ := newMockClient(t, &mockHandler{})
	_, err := c.PostOrderV2(nil, OrderTypeGTC, false, false)
	if err == nil {
		t.Error("expected error for nil order")
	}
}

// TestPostOrdersV2RejectsNilElement —— 批量提交里 args[i].Order 为 nil 时,
// 应当 error 并带上 index,而不是 nil pointer panic。
func TestPostOrdersV2RejectsNilElement(t *testing.T) {
	c, _ := newMockClient(t, &mockHandler{})
	c.SetCachedOrderVersion(2)
	// 先构一个合法 order
	order, err := c.CreateOrderV2(&OrderArgsV2{TokenID: "12345", Price: 0.5, Size: 10, Side: BUY}, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// 第 0 个合法,第 1 个 nil
	_, err = c.PostOrdersV2([]PostOrdersArgsV2{
		{Order: order, OrderType: OrderTypeGTC},
		{Order: nil, OrderType: OrderTypeGTC},
	})
	if err == nil {
		t.Fatal("expected error for nil order in batch")
	}
	if !strings.Contains(err.Error(), "[1]") {
		t.Errorf("error should mention index 1, got %v", err)
	}
}

// TestCreateBuilderAPIKeyCredsNoBody —— Python V2 是无 body POST,
// 我们的 SDK 不应该发任何 body(更不应该塞 builder_code 进 body)。
// 返回的是 raw 响应,由 ParseBuilderApiKey 容错解析。
func TestCreateBuilderAPIKeyCredsNoBody(t *testing.T) {
	var seenBody []byte
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/builder-api-key", func(w http.ResponseWriter, r *http.Request) {
		seenBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		// V2 Python 类型用 key/secret/passphrase 命名
		json.NewEncoder(w).Encode(map[string]interface{}{
			"key":        "bk-123",
			"secret":     "secret",
			"passphrase": "pass",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	creds := &ApiCreds{APIKey: "k", APISecret: testSecret, APIPassphrase: "p"}
	c, _ := NewClobClient(srv.URL, 137, v2TestPK, creds, nil, "")
	raw, err := c.CreateBuilderAPIKeyCreds()
	if err != nil {
		t.Fatalf("create builder key: %v", err)
	}
	if len(seenBody) != 0 {
		t.Errorf("expected empty body, got %q", string(seenBody))
	}
	parsed, err := ParseBuilderApiKey(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Key != "bk-123" || parsed.Secret != "secret" || parsed.Passphrase != "pass" {
		t.Errorf("parsed = %+v", parsed)
	}
}

// TestCreateBuilderAPIKeyTypedWrapper —— typed 便捷方法应直接拿到 *BuilderApiKey,
// 不需要调用方手动 ParseBuilderApiKey。
func TestCreateBuilderAPIKeyTypedWrapper(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/builder-api-key", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"key": "bk-xyz", "secret": "S", "passphrase": "P",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	creds := &ApiCreds{APIKey: "k", APISecret: testSecret, APIPassphrase: "p"}
	c, _ := NewClobClient(srv.URL, 137, v2TestPK, creds, nil, "")

	got, err := c.CreateBuilderAPIKey()
	if err != nil {
		t.Fatalf("typed: %v", err)
	}
	if got.Key != "bk-xyz" || got.Secret != "S" || got.Passphrase != "P" {
		t.Errorf("typed result mismatch: %+v", got)
	}
}

// TestParseBuilderApiKeyAcceptsLegacyFieldNames —— 兼容旧字段名 api_key/api_secret/api_passphrase。
func TestParseBuilderApiKeyAcceptsLegacyFieldNames(t *testing.T) {
	raw := map[string]interface{}{
		"api_key":        "k",
		"api_secret":     "s",
		"api_passphrase": "p",
	}
	parsed, err := ParseBuilderApiKey(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Key != "k" || parsed.Secret != "s" || parsed.Passphrase != "p" {
		t.Errorf("legacy field names not honored: %+v", parsed)
	}
}

// TestParseBuilderApiKeyRejectsMissingKey —— 缺 key 必须报错,不能返回零值。
func TestParseBuilderApiKeyRejectsMissingKey(t *testing.T) {
	if _, err := ParseBuilderApiKey(map[string]interface{}{"secret": "s"}); err == nil {
		t.Error("expected error when key missing")
	}
	if _, err := ParseBuilderApiKey("not a map"); err == nil {
		t.Error("expected error when input is not a map")
	}
}

// TestParseBuilderApiKeyRequiresAllThreeFields —— secret 或 passphrase 任一缺失
// 都必须报错。它们在 create 路径只返回一次,缺一个就再也拿不回来。
func TestParseBuilderApiKeyRequiresAllThreeFields(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]interface{}
		want string
	}{
		{"missing_secret", map[string]interface{}{"key": "K", "passphrase": "P"}, "secret"},
		{"missing_passphrase", map[string]interface{}{"key": "K", "secret": "S"}, "passphrase"},
		{"missing_key_and_secret", map[string]interface{}{"passphrase": "P"}, "key"},
		{"empty_secret_string", map[string]interface{}{"key": "K", "secret": "", "passphrase": "P"}, "secret"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseBuilderApiKey(c.in)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q should mention missing field %q", err.Error(), c.want)
			}
		})
	}
}

// TestParseBuilderApiKeyErrorDoesNotLeakCredentials —— 错误信息绝不能包含
// secret / passphrase 的实际值。这条防御调用方 log.Fatal(err) 时把凭证写到
// 日志 / Sentry / stderr。
func TestParseBuilderApiKeyErrorDoesNotLeakCredentials(t *testing.T) {
	sensitive := "super-secret-do-not-leak-9f8e7d6c5b4a"
	// 故意构造一个会失败的输入(缺 key),但带有 secret/passphrase。
	_, err := ParseBuilderApiKey(map[string]interface{}{
		"secret":     sensitive,
		"passphrase": sensitive,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), sensitive) {
		t.Errorf("error message leaked credential value:\n  %s", err.Error())
	}
}

// TestGetBuilderTradesNoAuth —— V2 builder trades 是 unauthenticated,
// 我们的 SDK 不应该发 POLY_API_KEY / POLY_SIGNATURE 等 header。
func TestGetBuilderTradesNoAuth(t *testing.T) {
	var sawAuthHeader bool
	var sawBuilderCode string
	mux := http.NewServeMux()
	mux.HandleFunc("/builder/trades", func(w http.ResponseWriter, r *http.Request) {
		// 任何一个 POLY_* header 都说明 SDK 加了 L2 auth(错的)
		for _, h := range []string{PolyAPIKey, PolySignature, PolyPassphrase} {
			if r.Header.Get(h) != "" {
				sawAuthHeader = true
			}
		}
		sawBuilderCode = r.URL.Query().Get("builder_code")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data":        []interface{}{},
			"next_cursor": EndCursor,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	creds := &ApiCreds{APIKey: "k", APISecret: testSecret, APIPassphrase: "p"}
	c, _ := NewClobClient(srv.URL, 137, v2TestPK, creds, nil, "")
	bc := "0xabc0000000000000000000000000000000000000000000000000000000000001"
	_, err := c.GetBuilderTrades(&BuilderTradeParams{BuilderCode: bc}, "")
	if err != nil {
		t.Fatalf("get builder trades: %v", err)
	}
	if sawAuthHeader {
		t.Error("V2 builder trades should not send L2 auth headers")
	}
	if sawBuilderCode != bc {
		t.Errorf("builder_code query missing/wrong: %q", sawBuilderCode)
	}
}

// TestGetBuilderTradesEncodesSpecialChars —— 即便过滤字段含 & 空格 = 等
// 特殊字符,URL 必须正确 encode,server 端拿到的还是原值,不会被破成多个参数。
func TestGetBuilderTradesEncodesSpecialChars(t *testing.T) {
	var got url.Values
	mux := http.NewServeMux()
	mux.HandleFunc("/builder/trades", func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data":        []interface{}{},
			"next_cursor": EndCursor,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	creds := &ApiCreds{APIKey: "k", APISecret: testSecret, APIPassphrase: "p"}
	c, _ := NewClobClient(srv.URL, 137, v2TestPK, creds, nil, "")

	weird := "a&b=c d#e"
	_, err := c.GetBuilderTrades(&BuilderTradeParams{
		BuilderCode:  "0xabc",
		Market:       weird,
		MakerAddress: "0xDEAD",
	}, "")
	if err != nil {
		t.Fatalf("get builder trades: %v", err)
	}
	if got.Get("market") != weird {
		t.Errorf("market mis-encoded: got %q want %q", got.Get("market"), weird)
	}
	if got.Get("maker_address") != "0xDEAD" {
		t.Errorf("maker_address mis-encoded: %q", got.Get("maker_address"))
	}
	if got.Get("builder_code") != "0xabc" {
		t.Errorf("builder_code mis-encoded: %q", got.Get("builder_code"))
	}
}

// TestGetBuilderTradesRequiresBuilderCode —— 没有 builder_code 必须直接报错。
func TestGetBuilderTradesRequiresBuilderCode(t *testing.T) {
	c, _ := newMockClient(t, &mockHandler{})
	if _, err := c.GetBuilderTrades(nil, ""); err == nil {
		t.Error("expected error when params nil")
	}
	if _, err := c.GetBuilderTrades(&BuilderTradeParams{}, ""); err == nil {
		t.Error("expected error when builder_code empty")
	}
	if _, err := c.GetBuilderTrades(&BuilderTradeParams{BuilderCode: Bytes32Zero}, ""); err == nil {
		t.Error("expected error when builder_code is zero")
	}
}

// TestCreateMarketOrderV2DoesNotMutateArgs —— 调用方传入的 args 必须保持不变,
// 即使 SDK 内部按 orderbook 算出了价格 / 按全局 builderCode 补 default。
func TestCreateMarketOrderV2DoesNotMutateArgs(t *testing.T) {
	c, _ := newMockClient(t, &mockHandler{})
	c.SetCachedOrderVersion(2)
	c.SetBuilderConfig(&BuilderConfig{BuilderCode: "0xabc0000000000000000000000000000000000000000000000000000000000001"})

	args := &MarketOrderArgsV2{
		TokenID:   "12345",
		Amount:    100,
		Side:      BUY,
		OrderType: OrderTypeFOK,
		// Price=0 → SDK 会算市价 0.55
		// BuilderCode 空 → SDK 会用 BuilderConfig 的全局值
	}
	originalPrice := args.Price
	originalBuilder := args.BuilderCode

	if _, err := c.CreateMarketOrderV2(args, nil); err != nil {
		t.Fatalf("market v2: %v", err)
	}
	if args.Price != originalPrice {
		t.Errorf("args.Price was mutated: %v → %v", originalPrice, args.Price)
	}
	if args.BuilderCode != originalBuilder {
		t.Errorf("args.BuilderCode was mutated: %q → %q", originalBuilder, args.BuilderCode)
	}
}

// TestGetClobMarketInfoErrorsOnEmptyTokens — 服务端返回缺 tokens 或空数组
// 时,必须直接 error,不能静默写入空缓存导致 fee_info 缺失。
func TestGetClobMarketInfoErrorsOnEmptyTokens(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/clob-markets/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// 故意返回不带 "t" 字段的响应
		json.NewEncoder(w).Encode(map[string]interface{}{"mts": "0.01"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	creds := &ApiCreds{APIKey: "k", APISecret: testSecret, APIPassphrase: "p"}
	c, _ := NewClobClient(srv.URL, 137, v2TestPK, creds, nil, "")
	if _, err := c.GetClobMarketInfo("cond-X"); err == nil {
		t.Error("expected error when tokens missing")
	}
}

// TestAdjustBuyAmountForBalanceErrorsWithoutFeeInfo — fee_info 缺失时,
// adjustBuyAmountForBalance 必须报错,不能默默按 fee=0 调整。
func TestAdjustBuyAmountForBalanceErrorsWithoutFeeInfo(t *testing.T) {
	// market_by_token 返回正常 condition_id,但 clob-markets 返回空 tokens
	mux := http.NewServeMux()
	mux.HandleFunc("/markets-by-token/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"condition_id": "cid-X"})
	})
	mux.HandleFunc("/clob-markets/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"mts": "0.01"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	creds := &ApiCreds{APIKey: "k", APISecret: testSecret, APIPassphrase: "p"}
	c, _ := NewClobClient(srv.URL, 137, v2TestPK, creds, nil, "")

	_, err := c.adjustBuyAmountForBalance("token-X", 100, 0.5, 50, "")
	if err == nil {
		t.Error("expected error from adjustBuyAmountForBalance when fee_info missing")
	}
}

// TestCreateOrderV2RoundsPriceBeforeFeeAdjust 验证:用户传 price=0.555 + tick=0.01
// 时,SDK 先把 price round_normal 到 0.56,然后 fee 计算 + build 都用 0.56,
// 而不是 fee 按 0.555 估算、签订单按 0.56。
//
// 期望:size=100, price=0.555(被 round 到 0.56), balance=1000(够花)→
//   makerAmount = 100 * 0.56 = 56 USDC = 56_000_000
func TestCreateOrderV2RoundsPriceBeforeFeeAdjust(t *testing.T) {
	c, _ := newMockClient(t, &mockHandler{tickSize: "0.01"})
	c.SetCachedOrderVersion(2)
	bal := 1000.0
	order, err := c.CreateOrderV2(&OrderArgsV2{
		TokenID:         "12345",
		Price:           0.555, // 会被 round 到 0.56
		Size:            100,
		Side:            BUY,
		UserUsdcBalance: &bal,
	}, nil)
	if err != nil {
		t.Fatalf("create v2: %v", err)
	}
	// makerAmount 应该是 100 * 0.56 = 56_000_000(不是 100*0.555 = 55_500_000)
	if order.MakerAmount.String() != "56000000" {
		t.Errorf("makerAmount=%s, want 56000000 (price must be rounded to 0.56)", order.MakerAmount.String())
	}
}

// TestCreateOrderV2WithUserUsdcBalance — BUY 限价单 + 提供 UserUsdcBalance,
// SDK 自动按市场费率公式缩小 size,与 Python _adjust_buy_amount_for_balance 一致。
//
// 设定:
//   amount = 100 * 0.5 = 50 USDC,price=0.5
//   balance = 10 USDC (远不够)
//   fee_rate = 0.02, exp = 1, no builder code, slippage = 0
// 公式 platform_fee_rate = 0.02 * (0.5 * 0.5)^1 = 0.005
// 平台费 = (min(50, 10) / 0.5) * 0.005 = 0.1
// 总成本 = 50 + 0.1 = 50.1 > 10,所以 adjusted = 10 - 0.1 = 9.9 USDC
// 调整后 size = 9.9 / 0.5 = 19.8 shares
// makerAmount = round(9.9 * 1e6) = 9_900_000
func TestCreateOrderV2WithUserUsdcBalance(t *testing.T) {
	c, _ := newMockClient(t, &mockHandler{})
	c.SetCachedOrderVersion(2)
	balance := 10.0
	order, err := c.CreateOrderV2(&OrderArgsV2{
		TokenID:         "12345",
		Price:           0.5,
		Size:            100, // 期望买 100 share
		Side:            BUY,
		UserUsdcBalance: &balance,
	}, nil)
	if err != nil {
		t.Fatalf("create v2: %v", err)
	}
	// 期望 makerAmount = 9_900_000(=9.9 USDC),size 被自动缩小
	if order.MakerAmount.String() != "9900000" {
		t.Errorf("makerAmount=%s, want 9900000 (auto-adjusted by balance)", order.MakerAmount.String())
	}
}

// TestCreateOrderV2PolyProxyMakerIsFunderSignerIsEOA
// PolyProxy (Magic/Email) 路径下,经 Polymarket 网页真实 POST /order 抓包验证:
//   - maker  = Deposit Wallet (funder)
//   - signer = EOA (签名者私钥派生的 ECDSA 地址,= API key 关联地址)
//   - signatureType = 1
func TestCreateOrderV2PolyProxyMakerIsFunderSignerIsEOA(t *testing.T) {
	creds := &ApiCreds{APIKey: "k", APISecret: testSecret, APIPassphrase: "p"}
	sig := SigTypePolyProxy
	funder := "0x1234567890123456789012345678901234567890"
	h := &mockHandler{}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, _ := NewClobClient(srv.URL, 137, v2TestPK, creds, &sig, funder)
	c.SetCachedOrderVersion(2)

	order, err := c.CreateOrderV2(&OrderArgsV2{
		TokenID: "12345", Price: 0.5, Size: 10, Side: BUY,
	}, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !strings.EqualFold(order.Maker.Hex(), funder) {
		t.Errorf("maker=%s, want funder=%s", order.Maker.Hex(), funder)
	}
	if !strings.EqualFold(order.Signer.Hex(), v2TestAddr) {
		t.Errorf("signer=%s, want EOA=%s", order.Signer.Hex(), v2TestAddr)
	}
}

// TestCreateOrderV2Poly1271MakerAndSignerAreFunder
// POLY_1271 路径下:
//   - maker  = Deposit Wallet 合约地址(funder)
//   - signer = Deposit Wallet 合约地址(funder)同一个;链上 isValidSignature
//     由合约 delegate 到 owner EOA 验证
//
// 对照 py-clob-client-v2._v2_order_signer 一致。
func TestCreateOrderV2Poly1271MakerAndSignerAreFunder(t *testing.T) {
	creds := &ApiCreds{APIKey: "k", APISecret: testSecret, APIPassphrase: "p"}
	sig := SigTypePoly1271
	depositWallet := "0x1234567890123456789012345678901234567890"
	h := &mockHandler{}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := NewClobClient(srv.URL, 137, v2TestPK, creds, &sig, depositWallet)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	c.SetCachedOrderVersion(2)

	order, err := c.CreateOrderV2(&OrderArgsV2{
		TokenID: "12345",
		Price:   0.5,
		Size:    10,
		Side:    BUY,
	}, nil)
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if !strings.EqualFold(order.Maker.Hex(), depositWallet) {
		t.Errorf("maker=%s, want depositWallet=%s", order.Maker.Hex(), depositWallet)
	}
	if !strings.EqualFold(order.Signer.Hex(), depositWallet) {
		t.Errorf("signer=%s, want depositWallet=%s", order.Signer.Hex(), depositWallet)
	}
	if order.SignatureType != V2SigTypePoly1271FromBuilder() {
		t.Errorf("signatureType=%d, want %d", order.SignatureType, V2SigTypePoly1271FromBuilder())
	}
	if len(order.Signature) < 2+2*(65+32+32+2) {
		t.Errorf("POLY_1271 signature too short: %d hex chars", len(order.Signature))
	}
}

// V2SigTypePoly1271FromBuilder 是 builder 包内 V2SigTypePoly1271 的镜像
// (避免在测试里 import builder 包的常量直接比较)。
func V2SigTypePoly1271FromBuilder() uint8 { return 3 }

// TestIsOrderVersionMismatchWithNestedErrorObject — error 字段是嵌套对象
// (而非字符串)时,IsOrderVersionMismatch 应序列化后字符串匹配。
func TestIsOrderVersionMismatchWithNestedErrorObject(t *testing.T) {
	resp := map[string]interface{}{
		"error": map[string]interface{}{
			"code":    "order_version_mismatch",
			"message": "expected v2",
			"details": []string{"a", "b"},
		},
	}
	if !IsOrderVersionMismatch(resp) {
		t.Error("nested object error should be detected as version mismatch")
	}
	resp2 := map[string]interface{}{
		"error": map[string]interface{}{
			"code": "something_else",
		},
	}
	if IsOrderVersionMismatch(resp2) {
		t.Error("unrelated nested error should NOT be flagged")
	}
}
