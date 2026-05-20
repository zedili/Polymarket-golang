package polymarket

import (
	"context"
	"encoding/json"
	"fmt"

	obuilder "github.com/0xNetuser/Polymarket-golang/polymarket/order_builder"
)

// ============================================================
//  Ctx-aware variants of the hottest public methods.
//
//  这些方法跟原版同名 + Ctx 后缀,区别只有第一个参数是 context.Context。
//  原版方法保留向后兼容,内部一律改成 `XxxCtx(context.Background(), ...)`。
//  bot 调用方常常需要在网络挂掉时取消 HTTP 请求(避免阻塞),Ctx 变种就是为此。
//
//  注意:CreateOrderV2 / CreateMarketOrderV2 不做网络 IO(只签名 + 本地编码),
//  所以不需要 Ctx 变种。CreateAndPostOrderV2 等做了 POST 的便利方法才需要。
// ============================================================

// CancelCtx —— Cancel 的 ctx 版本。
func (c *ClobClient) CancelCtx(ctx context.Context, orderID string) (interface{}, error) {
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}
	body := map[string]string{"orderID": orderID}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cancel request: %w", err)
	}
	bodyStr := string(bodyJSON)
	requestArgs := &RequestArgs{Method: "DELETE", RequestPath: Cancel, Body: body, SerializedBody: &bodyStr}
	headers, err := CreateLevel2Headers(c.signer, c.creds, requestArgs)
	if err != nil {
		return nil, err
	}
	return c.httpClient.DeleteCtx(ctx, Cancel, headers, bodyStr)
}

// CancelOrdersCtx —— CancelOrders 的 ctx 版本。
func (c *ClobClient) CancelOrdersCtx(ctx context.Context, orderIDs []string) (interface{}, error) {
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}
	bodyJSON, err := json.Marshal(orderIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal order IDs: %w", err)
	}
	bodyStr := string(bodyJSON)
	requestArgs := &RequestArgs{Method: "DELETE", RequestPath: CancelOrders, Body: orderIDs, SerializedBody: &bodyStr}
	headers, err := CreateLevel2Headers(c.signer, c.creds, requestArgs)
	if err != nil {
		return nil, err
	}
	return c.httpClient.DeleteCtx(ctx, CancelOrders, headers, bodyStr)
}

// CancelAllCtx —— CancelAll 的 ctx 版本。
func (c *ClobClient) CancelAllCtx(ctx context.Context) (interface{}, error) {
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}
	requestArgs := &RequestArgs{Method: "DELETE", RequestPath: CancelAll}
	headers, err := CreateLevel2Headers(c.signer, c.creds, requestArgs)
	if err != nil {
		return nil, err
	}
	return c.httpClient.DeleteCtx(ctx, CancelAll, headers, nil)
}

// CancelMarketOrdersCtx —— CancelMarketOrders 的 ctx 版本。
func (c *ClobClient) CancelMarketOrdersCtx(ctx context.Context, market, assetID string) (interface{}, error) {
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}
	body := map[string]string{"market": market, "asset_id": assetID}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cancel request: %w", err)
	}
	bodyStr := string(bodyJSON)
	requestArgs := &RequestArgs{Method: "DELETE", RequestPath: CancelMarketOrders, Body: body, SerializedBody: &bodyStr}
	headers, err := CreateLevel2Headers(c.signer, c.creds, requestArgs)
	if err != nil {
		return nil, err
	}
	return c.httpClient.DeleteCtx(ctx, CancelMarketOrders, headers, bodyStr)
}

// PostOrderV2Ctx —— PostOrderV2 的 ctx 版本。**适合需要 cancel 超时的调用方**。
//
// 行为细节(包括 IsOrderVersionMismatch 后 force-refresh /version 缓存)与
// 原版完全一致。
func (c *ClobClient) PostOrderV2Ctx(ctx context.Context, order *obuilder.SignedOrderV2, orderType OrderType, postOnly, deferExec bool) (*PostOrderResultV2, error) {
	if order == nil {
		return nil, fmt.Errorf("order is nil")
	}
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}
	if postOnly && (orderType == OrderTypeFOK || orderType == OrderTypeFAK) {
		return nil, fmt.Errorf("post_only orders cannot be FOK or FAK")
	}
	body := order.ToJSONPayload(c.creds.APIKey, string(orderType), postOnly, deferExec)
	bodyStr, err := MarshalCompact(body)
	if err != nil {
		return nil, err
	}
	req := &RequestArgs{Method: "POST", RequestPath: PostOrder, Body: body, SerializedBody: &bodyStr}
	headers, err := CreateLevel2Headers(c.signer, c.creds, req)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.PostCtx(ctx, PostOrder, headers, bodyStr)
	if err != nil {
		return nil, err
	}
	if IsOrderVersionMismatch(resp) {
		c.ResolveOrderVersion(true)
	}
	return &PostOrderResultV2{Payload: body, Response: resp}, nil
}

// CreateAndPostOrderV2Ctx —— CreateAndPostOrderV2 的 ctx 版本。
//
// 注意:仅 POST 阶段受 ctx 约束。CreateOrderV2 是纯本地操作(签名 + 编码),
// ctx 在那里没意义。
func (c *ClobClient) CreateAndPostOrderV2Ctx(
	ctx context.Context,
	args *OrderArgsV2,
	options *PartialCreateOrderOptions,
	orderType OrderType,
	postOnly, deferExec bool,
) (*PostOrderResultV2, error) {
	if orderType == "" {
		orderType = OrderTypeGTC
	}
	result, err := c.retryOnVersionUpdate(func() (interface{}, error) {
		order, err := c.CreateOrderV2(args, options)
		if err != nil {
			return nil, err
		}
		return c.PostOrderV2Ctx(ctx, order, orderType, postOnly, deferExec)
	})
	if err != nil {
		return nil, err
	}
	if r, ok := result.(*PostOrderResultV2); ok {
		return r, nil
	}
	return nil, fmt.Errorf("unexpected result type from retry")
}

// CreateAndPostMarketOrderV2Ctx —— CreateAndPostMarketOrderV2 的 ctx 版本。
func (c *ClobClient) CreateAndPostMarketOrderV2Ctx(
	ctx context.Context,
	args *MarketOrderArgsV2,
	options *PartialCreateOrderOptions,
	orderType OrderType,
	deferExec bool,
) (*PostOrderResultV2, error) {
	if orderType == "" {
		orderType = OrderTypeFOK
	}
	result, err := c.retryOnVersionUpdate(func() (interface{}, error) {
		order, err := c.CreateMarketOrderV2(args, options)
		if err != nil {
			return nil, err
		}
		return c.PostOrderV2Ctx(ctx, order, orderType, false, deferExec)
	})
	if err != nil {
		return nil, err
	}
	if r, ok := result.(*PostOrderResultV2); ok {
		return r, nil
	}
	return nil, fmt.Errorf("unexpected result type from retry")
}

// GetBalanceAllowanceCtx —— GetBalanceAllowance 的 ctx 版本。
//
// signature_type 取自 client.sigType(参数未设时回填),与原版一致。
func (c *ClobClient) GetBalanceAllowanceCtx(ctx context.Context, params *BalanceAllowanceParams) (map[string]interface{}, error) {
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}
	if params == nil {
		params = &BalanceAllowanceParams{}
	}
	if params.SignatureType == nil || *params.SignatureType < 0 {
		sigType := c.sigType
		params.SignatureType = &sigType
	}
	url := AddBalanceAllowanceParamsToURL(c.host+GetBalanceAllowance, params)
	requestArgs := &RequestArgs{Method: "GET", RequestPath: GetBalanceAllowance}
	headers, err := CreateLevel2Headers(c.signer, c.creds, requestArgs)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.GetCtx(ctx, url[len(c.host):], headers)
	if err != nil {
		return nil, err
	}
	if m, ok := resp.(map[string]interface{}); ok {
		return m, nil
	}
	return nil, fmt.Errorf("invalid response format")
}
