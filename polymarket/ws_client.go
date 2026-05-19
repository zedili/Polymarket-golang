// Polymarket WebSocket client(market + user channels)
//
// 文档:
//   - https://docs.polymarket.com/market-data/websocket/overview
//   - https://docs.polymarket.com/market-data/websocket/market-channel
//   - https://docs.polymarket.com/market-data/websocket/user-channel
//
// 设计要点:
//   - 一个 WSClient 实例只跑一个频道(/ws/market 或 /ws/user),保持职责清晰
//   - 内部 goroutine 读 + ping 循环;dispatch 到强类型 handlers
//   - context 取消即 close;Run 阻塞返回 error
//   - 自动 reconnect 由调用方控制(Run 返回后调方决定重连节奏)
//   - 服务端要求每 10s 发一次 PING(plain text),收到 PONG (plain) 即可
package polymarket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Sentinel errors —— 上层 reconnect 决策时用 errors.Is 判别。
var (
	// ErrWSServerClose 服务端正常或异常关闭(非鉴权 1006)。重连通常有用。
	ErrWSServerClose = errors.New("ws: server closed connection")
	// ErrWSAuthRejected /ws/user 拿不合法凭证(builder key / 错的 derived key)时
	// 服务端会在连上 2s 内 1006 close。重连无意义,要换 creds。
	ErrWSAuthRejected = errors.New("ws: authentication rejected (likely wrong creds)")
	// ErrWSReadTimeout 超过 ReadDeadline 还没收到任何数据 —— 半开连接症状,应重连。
	ErrWSReadTimeout = errors.New("ws: read deadline exceeded (likely half-open connection)")
)

// ============================================================
//  Endpoint constants
// ============================================================

// WSBaseURL 默认 WS base。文档:wss://ws-subscriptions-clob.polymarket.com
const WSBaseURL = "wss://ws-subscriptions-clob.polymarket.com"

const (
	WSPathMarket = "/ws/market"
	WSPathUser   = "/ws/user"
)

// 默认 ping 周期(服务端期望 10s);读 deadline 是 ping 周期的 3.5 倍(留出
// 服务端 PONG 抖动 + 网络延迟)。SetPingInterval 可改。
const (
	defaultWSPingInterval     = 10 * time.Second
	defaultWSReadDeadlineMult = 3500 * time.Millisecond / time.Second // 即 3.5x
	defaultWSReadLimit        = int64(1 << 20)                       // 1 MiB,大 book 快照也够
)

// ============================================================
//  Event payloads —— 跟 doc 字段对齐
// ============================================================

// WSBookEvent 完整 orderbook 快照,刚订阅就会推一次,之后撮合后再推。
type WSBookEvent struct {
	EventType string           `json:"event_type"` // "book"
	AssetID   string           `json:"asset_id"`
	Market    string           `json:"market"`
	Bids      []WSBookLevel    `json:"bids"`
	Asks      []WSBookLevel    `json:"asks"`
	Timestamp string           `json:"timestamp"`
	Hash      string           `json:"hash"`
}

// WSBookLevel 单一价位 size。注意 size 是字符串(服务端原样)。
type WSBookLevel struct {
	Price string `json:"price"`
	Size  string `json:"size"`
}

// WSPriceChangeEvent 挂单 / 撤单 delta。
//
// price_changes 里如果 size == "0" 表示该价位被清空。
type WSPriceChangeEvent struct {
	EventType    string          `json:"event_type"`
	Market       string          `json:"market"`
	PriceChanges []WSPriceChange `json:"price_changes"`
	Timestamp    string          `json:"timestamp"`
}

type WSPriceChange struct {
	AssetID  string `json:"asset_id"`
	Price    string `json:"price"`
	Size     string `json:"size"`
	Side     string `json:"side"`
	Hash     string `json:"hash"`
	BestBid  string `json:"best_bid"`
	BestAsk  string `json:"best_ask"`
}

// WSLastTradePriceEvent 撮合成交后推。
type WSLastTradePriceEvent struct {
	EventType   string `json:"event_type"`
	AssetID     string `json:"asset_id"`
	Market      string `json:"market"`
	Price       string `json:"price"`
	Side        string `json:"side"`
	Size        string `json:"size"`
	FeeRateBps  string `json:"fee_rate_bps"`
	Timestamp   string `json:"timestamp"`
}

// WSTickSizeChangeEvent 价格触及极端档位(>0.96 或 <0.04)时 tick 收紧/扩大。
type WSTickSizeChangeEvent struct {
	EventType   string `json:"event_type"`
	AssetID     string `json:"asset_id"`
	Market      string `json:"market"`
	OldTickSize string `json:"old_tick_size"`
	NewTickSize string `json:"new_tick_size"`
	Timestamp   string `json:"timestamp"`
}

// WSTradeEvent (user channel) 撮合 / on-chain 状态变更。
type WSTradeEvent struct {
	EventType   string            `json:"event_type"` // "trade"
	ID          string            `json:"id"`
	Status      string            `json:"status"` // MATCHED / MINED / CONFIRMED / RETRYING / FAILED
	MakerOrders []WSMakerOrderRef `json:"maker_orders"`
	Market      string            `json:"market"`
	Outcome     string            `json:"outcome"`
	Side        string            `json:"side"`
	Size        string            `json:"size"`
	Price       string            `json:"price"`
	Timestamp   string            `json:"timestamp"`
	TradeOwner  string            `json:"trade_owner"`
}

type WSMakerOrderRef struct {
	AssetID       string `json:"asset_id"`
	MatchedAmount string `json:"matched_amount"`
	OrderID       string `json:"order_id"`
	Outcome       string `json:"outcome"`
	Owner         string `json:"owner"`
	Price         string `json:"price"`
}

// WSOrderEvent (user channel) 自己订单的生命周期事件。
type WSOrderEvent struct {
	EventType    string `json:"event_type"` // "order"
	ID           string `json:"id"`
	Type         string `json:"type"` // PLACEMENT / UPDATE / CANCELLATION
	AssetID      string `json:"asset_id"`
	Market       string `json:"market"`
	Outcome      string `json:"outcome"`
	Side         string `json:"side"`
	OriginalSize string `json:"original_size"`
	SizeMatched  string `json:"size_matched"`
	Price        string `json:"price"`
	OrderOwner   string `json:"order_owner"`
	Timestamp    string `json:"timestamp"`
}

// ============================================================
//  Handler 接口
// ============================================================

// WSHandler 处理 WS 收到的事件。各方法可空 —— 没注册 = 丢弃。
//
// HandleUnknown 兜底:任何 event_type 不识别的消息走这里(包含 best_bid_ask /
// new_market / market_resolved 这些需要 custom_feature_enabled=true 才订得到
// 的事件)。raw 是原始 JSON,可自行 Unmarshal。
type WSHandler struct {
	OnBook           func(WSBookEvent)
	OnPriceChange    func(WSPriceChangeEvent)
	OnLastTradePrice func(WSLastTradePriceEvent)
	OnTickSizeChange func(WSTickSizeChangeEvent)
	OnTrade          func(WSTradeEvent)
	OnOrder          func(WSOrderEvent)
	HandleUnknown    func(eventType string, raw json.RawMessage)
}

// ============================================================
//  WSClient
// ============================================================

// WSClient 一个 Polymarket WebSocket 连接。
//
// Run() 阻塞,直到 ctx 取消、连接断开或者读 / 写出错 —— 之后返回 error 给调用方
// 让其决定是否重连。重连策略不内置(调用方常常想要 backoff 自己控)。
type WSClient struct {
	baseURL string
	path    string

	// subscribe payload。由 Subscribe* 方法构造。
	subscribePayload interface{}

	handler      *WSHandler
	dialer       *websocket.Dialer
	conn         *websocket.Conn
	writeMu      sync.Mutex
	pingInterval time.Duration // 0 = 默认
	readLimit    int64         // 0 = 默认
	connectedAt  time.Time
}

// NewMarketWSClient 创建 /ws/market 客户端(无认证)。
//
// assetIDs 必填:订阅哪些 token 的盘口。customFeature=true 时还能收到
// best_bid_ask / new_market / market_resolved(走 HandleUnknown 兜底)。
func NewMarketWSClient(assetIDs []string, customFeature bool, h *WSHandler) (*WSClient, error) {
	if len(assetIDs) == 0 {
		return nil, errors.New("market WS requires at least one asset_id")
	}
	if h == nil {
		return nil, errors.New("WSHandler is required")
	}
	return &WSClient{
		baseURL: WSBaseURL,
		path:    WSPathMarket,
		subscribePayload: map[string]interface{}{
			"assets_ids":             assetIDs,
			"type":                   "market",
			"custom_feature_enabled": customFeature,
		},
		handler: h,
		dialer:  websocket.DefaultDialer,
	}, nil
}

// NewUserWSClient 创建 /ws/user 客户端(需要 L2 凭证)。
//
// markets:condition_id 列表;ApiCreds 必须是 **EOA 派生** 的 user key
// (CreateOrDeriveAPIKey 的产物)。**Builder API key 不能用** —— 实测服务端会
// 立刻 close 连接(1006 abnormal closure)。这点文档没明确写,是实际测试出来的。
//
// 警告(文档):User channel 凭证只能在服务端用,绝对不要放到浏览器/客户端代码里。
func NewUserWSClient(creds *ApiCreds, conditionIDs []string, h *WSHandler) (*WSClient, error) {
	if creds == nil || creds.APIKey == "" || creds.APISecret == "" || creds.APIPassphrase == "" {
		return nil, errors.New("user WS requires non-empty ApiCreds")
	}
	if h == nil {
		return nil, errors.New("WSHandler is required")
	}
	return &WSClient{
		baseURL: WSBaseURL,
		path:    WSPathUser,
		subscribePayload: map[string]interface{}{
			"auth": map[string]interface{}{
				"apiKey":     creds.APIKey,
				"secret":     creds.APISecret,
				"passphrase": creds.APIPassphrase,
			},
			"markets": conditionIDs,
			"type":    "user",
		},
		handler: h,
		dialer:  websocket.DefaultDialer,
	}, nil
}

// WithBaseURL 覆盖 WS base URL(测试用)。
func (c *WSClient) WithBaseURL(url string) *WSClient {
	c.baseURL = url
	return c
}

// WithPingInterval 修改 ping 周期(默认 10s)。读 deadline 会自动设为 ping*3.5。
// 多实例同时跑时,加一点抖动避免所有 bot 同时 ping。
func (c *WSClient) WithPingInterval(d time.Duration) *WSClient {
	if d > 0 {
		c.pingInterval = d
	}
	return c
}

// WithReadLimit 修改单帧最大字节(默认 1 MiB)。
func (c *WSClient) WithReadLimit(n int64) *WSClient {
	if n > 0 {
		c.readLimit = n
	}
	return c
}

func (c *WSClient) ping() time.Duration {
	if c.pingInterval > 0 {
		return c.pingInterval
	}
	return defaultWSPingInterval
}

func (c *WSClient) readDeadline() time.Duration {
	// 3.5x ping。给服务端 PONG 抖动 + 跨洋网络延迟留余量。
	return c.ping()*7/2
}

func (c *WSClient) limit() int64 {
	if c.readLimit > 0 {
		return c.readLimit
	}
	return defaultWSReadLimit
}

// Run 连 WS、subscribe、阻塞读消息直到出错或 ctx 取消。
//
// 返回的 error 区分语义,上层 reconnect 用 errors.Is:
//   - ctx.Err() (Canceled/DeadlineExceeded): 主动停
//   - ErrWSAuthRejected: 凭证不对,**别重连**
//   - ErrWSReadTimeout:  半开连接,**重连**
//   - ErrWSServerClose:  server 正常或异常关闭,**重连**
//   - 其他 wrap 错误:    一般也是 transient,重连
func (c *WSClient) Run(ctx context.Context) error {
	url := c.baseURL + c.path
	conn, _, err := c.dialer.DialContext(ctx, url, http.Header{})
	if err != nil {
		return fmt.Errorf("ws dial %s: %w", url, err)
	}
	c.conn = conn
	c.connectedAt = time.Now()
	defer conn.Close()

	// 读 deadline + 单帧大小上限
	conn.SetReadLimit(c.limit())
	resetReadDeadline := func() {
		_ = conn.SetReadDeadline(time.Now().Add(c.readDeadline()))
	}
	resetReadDeadline()

	// 服务端会发 ws-level Pong frame 回应我们的 ws-level Ping。我们用文本 "PING"
	// (Polymarket 自定的应用层 ping),所以这里同时把两个机制的 deadline 重置 hook
	// 都装上 —— 安全冗余。
	conn.SetPongHandler(func(string) error { resetReadDeadline(); return nil })
	conn.SetPingHandler(func(appData string) error {
		// gorilla 默认会写 pong;只更新 deadline
		resetReadDeadline()
		return nil
	})

	// subscribe (JSON)
	if err := c.writeJSON(c.subscribePayload); err != nil {
		return fmt.Errorf("ws subscribe: %w", err)
	}

	// reader: 读到 deadline / server close / 网络错都退出
	readErr := make(chan error, 1)
	go func() { readErr <- c.readLoop(resetReadDeadline) }()

	// ping loop — 文档要求 ~10s 一次 PING(纯文本,不是 ws Ping frame)。
	// 加 ±10% jitter,防多个 client 同步 ping 给服务端造成尖峰。
	ping := c.ping()
	jitterFn := func() time.Duration {
		j := time.Duration(rand.Int63n(int64(ping) / 5)) // 0~20% 偏移
		return ping - ping/10 + j                        // 中心 90%~110%
	}
	pingTimer := time.NewTimer(jitterFn())
	defer pingTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			// 主动关闭。给 server 发个 close 帧。
			_ = conn.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
				time.Now().Add(2*time.Second))
			return ctx.Err()
		case <-pingTimer.C:
			c.writeMu.Lock()
			err := conn.WriteMessage(websocket.TextMessage, []byte("PING"))
			c.writeMu.Unlock()
			if err != nil {
				return fmt.Errorf("ws write PING: %w", err)
			}
			pingTimer.Reset(jitterFn())
		case err := <-readErr:
			return err
		}
	}
}

func (c *WSClient) writeJSON(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

func (c *WSClient) readLoop(resetDeadline func()) error {
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			// Read deadline:半开连接症状,返回 sentinel
			var netErr interface{ Timeout() bool }
			if errors.As(err, &netErr) && netErr.Timeout() {
				return ErrWSReadTimeout
			}
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return ErrWSServerClose
			}
			// 1006 abnormal close 在连上 2s 内出现 → 极大概率 server 拒鉴权
			if websocket.IsCloseError(err, websocket.CloseAbnormalClosure) ||
				websocket.IsUnexpectedCloseError(err, websocket.CloseNoStatusReceived) {
				if time.Since(c.connectedAt) < 3*time.Second {
					return fmt.Errorf("%w: %v", ErrWSAuthRejected, err)
				}
				return fmt.Errorf("%w: %v", ErrWSServerClose, err)
			}
			return fmt.Errorf("ws read: %w", err)
		}
		// 收到任何数据 → 半开检测重置
		resetDeadline()
		// PONG 文本响应直接丢
		if len(msg) == 4 && string(msg) == "PONG" {
			continue
		}
		c.dispatch(msg)
	}
}

// dispatch 把消息按 event_type 路由到 handler。
//
// 服务端的"一条消息"可能是:
//   - 单 object: `{"event_type":"book",...}`
//   - JSON 数组: `[{"event_type":"book",...}, {...}]`(批量)
//
// 文档没明确,但实际推送中两种都见到过,所以这里两种都收。
func (c *WSClient) dispatch(raw []byte) {
	trim := skipSpace(raw)
	if len(trim) == 0 {
		return
	}
	if trim[0] == '[' {
		var batch []json.RawMessage
		if err := json.Unmarshal(trim, &batch); err == nil {
			for _, m := range batch {
				c.dispatchOne(m)
			}
			return
		}
	}
	c.dispatchOne(trim)
}

func (c *WSClient) dispatchOne(raw json.RawMessage) {
	var head struct {
		EventType string `json:"event_type"`
	}
	if err := json.Unmarshal(raw, &head); err != nil || head.EventType == "" {
		// 顶层 decode 失败:走 HandleUnknown 让调用方至少能 log
		if c.handler.HandleUnknown != nil {
			c.handler.HandleUnknown("", raw)
		}
		return
	}

	// tryDecode 把 typed unmarshal 错也送到 HandleUnknown(关键!Polymarket
	// schema 漂移时,trade/order 事件不能静默丢)。
	tryDecode := func(handler func(), typed interface{}) {
		if handler == nil {
			return
		}
		if err := json.Unmarshal(raw, typed); err != nil {
			if c.handler.HandleUnknown != nil {
				c.handler.HandleUnknown(head.EventType+":decode_error", raw)
			}
			return
		}
		handler()
	}

	switch head.EventType {
	case "book":
		var e WSBookEvent
		tryDecode(func() { c.handler.OnBook(e) }, &e)
	case "price_change":
		var e WSPriceChangeEvent
		tryDecode(func() { c.handler.OnPriceChange(e) }, &e)
	case "last_trade_price":
		var e WSLastTradePriceEvent
		tryDecode(func() { c.handler.OnLastTradePrice(e) }, &e)
	case "tick_size_change":
		var e WSTickSizeChangeEvent
		tryDecode(func() { c.handler.OnTickSizeChange(e) }, &e)
	case "trade":
		var e WSTradeEvent
		tryDecode(func() { c.handler.OnTrade(e) }, &e)
	case "order":
		var e WSOrderEvent
		tryDecode(func() { c.handler.OnOrder(e) }, &e)
	default:
		if c.handler.HandleUnknown != nil {
			c.handler.HandleUnknown(head.EventType, raw)
		}
	}
}

// skipSpace 跳过前导空白(JSON 允许)。
func skipSpace(b []byte) []byte {
	for len(b) > 0 {
		switch b[0] {
		case ' ', '\t', '\r', '\n':
			b = b[1:]
		default:
			return b
		}
	}
	return b
}
