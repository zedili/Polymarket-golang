package polymarket

import (
	"context"
	"encoding/json"
	"fmt"
)

// ============================================================
//  Typed responses for hot endpoints.
//
//  原本 GetOrders / GetTrades / GetOrder / Cancel 等返回 `interface{}` 或
//  `[]interface{}`,调用方要手写 map[string]interface{} 取字段,容易写错且没
//  IDE 自动补全。这里加一组 `*Typed` 变种,内部走原方法 + 一次 JSON 回弹解码,
//  把 interface{} 变成强类型。
//
//  字段命名与 Polymarket V2 OpenAPI 一致(见 /api-reference/trade/*)。
//  解码用了"json.Marshal then json.Unmarshal"的桥接方式 —— 多花一次序列化但
//  足够通用,不需要 reflect 手动 map。Hot 路径调用方真嫌慢可以继续用原版自己解。
// ============================================================

// OpenOrder 是 /data/orders 返回的单条订单结构。
//
// 字段对齐 V2 服务端 `Order` schema。一些字段在不同订单状态下可能为空。
type OpenOrder struct {
	ID             string  `json:"id"`             // 订单 ID(签名 hash)
	Status         string  `json:"status"`         // LIVE / CANCELED / MATCHED / ...
	Owner          string  `json:"owner"`          // API key owner
	MakerAddress   string  `json:"maker_address"`  // funder / proxy
	Market         string  `json:"market"`         // condition_id
	AssetID        string  `json:"asset_id"`       // token_id (字符串十进制)
	Side           string  `json:"side"`           // BUY / SELL
	OriginalSize   string  `json:"original_size"`  // 下单时 size(字符串小数)
	SizeMatched   string  `json:"size_matched"`   // 已成交数量
	Price          string  `json:"price"`          // limit 价
	Outcome        string  `json:"outcome"`        // Yes / No / Up / Down ...
	Expiration     string  `json:"expiration"`     // unix 秒(字符串)
	OrderType      string  `json:"order_type"`     // GTC / GTD / FOK / FAK
	AssociateTrades []string `json:"associate_trades,omitempty"`
	CreatedAt      int64    `json:"created_at"`
}

// Trade 是 /data/trades 返回的单条成交。
type Trade struct {
	ID                  string   `json:"id"`
	TakerOrderID        string   `json:"taker_order_id"`
	Market              string   `json:"market"`
	AssetID             string   `json:"asset_id"`
	Side                string   `json:"side"`
	Size                string   `json:"size"`
	FeeRateBps          string   `json:"fee_rate_bps"`
	Price               string   `json:"price"`
	Status              string   `json:"status"` // MATCHED / MINED / CONFIRMED / RETRYING / FAILED
	MatchTime           string   `json:"match_time"`
	LastUpdate          string   `json:"last_update"`
	Outcome             string   `json:"outcome"`
	MakerAddress        string   `json:"maker_address"`
	Owner               string   `json:"owner"`
	TransactionHash     string   `json:"transaction_hash"`
	Bucket              string   `json:"bucket_index,omitempty"`
	MakerOrders         []MakerOrderRef `json:"maker_orders"`
	Type                string   `json:"type,omitempty"` // TAKER / MAKER
}

// MakerOrderRef 是 Trade.maker_orders 的元素。
type MakerOrderRef struct {
	OrderID       string `json:"order_id"`
	Owner         string `json:"owner"`
	MakerAddress  string `json:"maker_address"`
	MatchedAmount string `json:"matched_amount"`
	Price         string `json:"price"`
	FeeRateBps    string `json:"fee_rate_bps"`
	AssetID       string `json:"asset_id"`
	Outcome       string `json:"outcome"`
}

// CancelResult 是 DELETE /order / /orders / /cancel-all / /cancel-market-orders 返回。
//
// canceled 是被实际撤掉的 ID;not_canceled 是 key=ID,value=拒因(LIVE 已成交、
// 不存在、已是 cancelled 等)。
type CancelResult struct {
	Canceled    []string          `json:"canceled"`
	NotCanceled map[string]string `json:"not_canceled,omitempty"`
}

// ============================================================
//  Typed variants
// ============================================================

// GetOrdersTyped —— GetOrders 的强类型版本,自动 JSON 解码。
//
// 多花一次 json.Marshal+Unmarshal 桥接;不在乎额外 ~10µs 开销的场景请直接用
// GetOrders + 手动 type-assert。
func (c *ClobClient) GetOrdersTyped(params *OpenOrderParams, nextCursor string) ([]OpenOrder, error) {
	return c.GetOrdersTypedCtx(context.Background(), params, nextCursor)
}

// GetOrdersTypedCtx —— GetOrdersTyped 的 ctx 版本。
//
// 没有专门复制底层逻辑 —— 走 GetOrders 然后桥接解码。ctx 只能在 GetOrdersCtx
// 暴露后才能真生效;当前 GetOrders 内部已经用 ctx-aware HTTP,这里通过外层
// goroutine 配合 ctx.Done 提早返回。
func (c *ClobClient) GetOrdersTypedCtx(ctx context.Context, params *OpenOrderParams, nextCursor string) ([]OpenOrder, error) {
	type result struct {
		raw []interface{}
		err error
	}
	ch := make(chan result, 1)
	go func() {
		raw, err := c.GetOrders(params, nextCursor)
		ch <- result{raw, err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return nil, r.err
		}
		return decodeOrdersList(r.raw)
	}
}

// GetTradesTyped —— GetTrades 的强类型版本。
func (c *ClobClient) GetTradesTyped(params *TradeParams, nextCursor string) ([]Trade, error) {
	return c.GetTradesTypedCtx(context.Background(), params, nextCursor)
}

// GetTradesTypedCtx —— GetTradesTyped 的 ctx 版本。
func (c *ClobClient) GetTradesTypedCtx(ctx context.Context, params *TradeParams, nextCursor string) ([]Trade, error) {
	type result struct {
		raw []interface{}
		err error
	}
	ch := make(chan result, 1)
	go func() {
		raw, err := c.GetTrades(params, nextCursor)
		ch <- result{raw, err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return nil, r.err
		}
		var out []Trade
		for i, item := range r.raw {
			var t Trade
			if err := jsonRoundtrip(item, &t); err != nil {
				return nil, fmt.Errorf("decode trade[%d]: %w", i, err)
			}
			out = append(out, t)
		}
		return out, nil
	}
}

// GetOrderTyped —— GetOrder 的强类型版本。返回单条订单。
func (c *ClobClient) GetOrderTyped(orderID string) (*OpenOrder, error) {
	raw, err := c.GetOrder(orderID)
	if err != nil {
		return nil, err
	}
	var o OpenOrder
	if err := jsonRoundtrip(raw, &o); err != nil {
		return nil, fmt.Errorf("decode order: %w", err)
	}
	return &o, nil
}

// CancelTyped —— Cancel 的强类型版本。
func (c *ClobClient) CancelTyped(orderID string) (*CancelResult, error) {
	raw, err := c.Cancel(orderID)
	if err != nil {
		return nil, err
	}
	return decodeCancel(raw)
}

// CancelOrdersTyped —— CancelOrders 的强类型版本。
func (c *ClobClient) CancelOrdersTyped(orderIDs []string) (*CancelResult, error) {
	raw, err := c.CancelOrders(orderIDs)
	if err != nil {
		return nil, err
	}
	return decodeCancel(raw)
}

// CancelAllTyped —— CancelAll 的强类型版本。
func (c *ClobClient) CancelAllTyped() (*CancelResult, error) {
	raw, err := c.CancelAll()
	if err != nil {
		return nil, err
	}
	return decodeCancel(raw)
}

// ============================================================
//  Helpers
// ============================================================

// jsonRoundtrip 用 Marshal+Unmarshal 把任意 interface{} 桥接到强类型 struct。
//
// 比 reflect 手动 copy 快(json.Marshal 写过 fastpath),适用于解码 SDK 客户端
// 已经把 server JSON unmarshal 成 map[string]interface{} 后,我们再二次 typed 化。
func jsonRoundtrip(src interface{}, dst interface{}) error {
	if src == nil {
		return fmt.Errorf("nil source")
	}
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

func decodeOrdersList(raw []interface{}) ([]OpenOrder, error) {
	var out []OpenOrder
	for i, item := range raw {
		var o OpenOrder
		if err := jsonRoundtrip(item, &o); err != nil {
			return nil, fmt.Errorf("decode order[%d]: %w", i, err)
		}
		out = append(out, o)
	}
	return out, nil
}

func decodeCancel(raw interface{}) (*CancelResult, error) {
	// 服务端可能返回 list(老 API)或 object 带 canceled/not_canceled
	switch v := raw.(type) {
	case nil:
		return &CancelResult{}, nil
	case []interface{}:
		// 老 API:直接是已取消 ID 数组
		ids := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				ids = append(ids, s)
			}
		}
		return &CancelResult{Canceled: ids}, nil
	default:
		var r CancelResult
		if err := jsonRoundtrip(raw, &r); err != nil {
			return nil, fmt.Errorf("decode cancel response: %w", err)
		}
		return &r, nil
	}
}
