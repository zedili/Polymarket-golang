package polymarket

import (
	"encoding/json"
	"fmt"
)

// 注意:V2 迁移完成后,本文件不再提供 PostOrder / PostOrders / PostOrderWithOptions
// (V1) 系列方法。下单走 V2:
//   - 单个限价/市价:CreateAndPostOrderV2 / CreateAndPostMarketOrderV2
//   - 单独构造/提交:CreateOrderV2 + PostOrderV2、CreateMarketOrderV2 + PostOrderV2
//   - 批量:PostOrdersV2
// 下面保留的是与订单版本无关的 L2 操作:取消、查询、余额、通知。

// Cancel 取消订单
// 需要L2认证
func (c *ClobClient) Cancel(orderID string) (interface{}, error) {
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}

	body := map[string]string{"orderID": orderID}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cancel request: %w", err)
	}
	bodyStr := string(bodyJSON)

	requestArgs := &RequestArgs{
		Method:        "DELETE",
		RequestPath:   Cancel,
		Body:          body,
		SerializedBody: &bodyStr,
	}

	headers, err := CreateLevel2Headers(c.signer, c.creds, requestArgs)
	if err != nil {
		return nil, err
	}

	return c.httpClient.Delete(Cancel, headers, bodyStr)
}

// CancelOrders 批量取消订单
// 需要L2认证
func (c *ClobClient) CancelOrders(orderIDs []string) (interface{}, error) {
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}

	bodyJSON, err := json.Marshal(orderIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal order IDs: %w", err)
	}
	bodyStr := string(bodyJSON)

	requestArgs := &RequestArgs{
		Method:        "DELETE",
		RequestPath:   CancelOrders,
		Body:          orderIDs,
		SerializedBody: &bodyStr,
	}

	headers, err := CreateLevel2Headers(c.signer, c.creds, requestArgs)
	if err != nil {
		return nil, err
	}

	return c.httpClient.Delete(CancelOrders, headers, bodyStr)
}

// CancelAll 取消所有订单
// 需要L2认证
func (c *ClobClient) CancelAll() (interface{}, error) {
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}

	requestArgs := &RequestArgs{
		Method:      "DELETE",
		RequestPath: CancelAll,
	}

	headers, err := CreateLevel2Headers(c.signer, c.creds, requestArgs)
	if err != nil {
		return nil, err
	}

	return c.httpClient.Delete(CancelAll, headers, nil)
}

// CancelMarketOrders 取消市场订单
// 需要L2认证
func (c *ClobClient) CancelMarketOrders(market, assetID string) (interface{}, error) {
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}

	body := map[string]string{
		"market":   market,
		"asset_id": assetID,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cancel request: %w", err)
	}
	bodyStr := string(bodyJSON)

	requestArgs := &RequestArgs{
		Method:        "DELETE",
		RequestPath:   CancelMarketOrders,
		Body:          body,
		SerializedBody: &bodyStr,
	}

	headers, err := CreateLevel2Headers(c.signer, c.creds, requestArgs)
	if err != nil {
		return nil, err
	}

	return c.httpClient.Delete(CancelMarketOrders, headers, bodyStr)
}

// GetOrders 获取订单列表
// 需要L2认证
func (c *ClobClient) GetOrders(params *OpenOrderParams, nextCursor string) ([]interface{}, error) {
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}

	if nextCursor == "" {
		nextCursor = "MA=="
	}

	requestArgs := &RequestArgs{
		Method:      "GET",
		RequestPath: Orders,
	}

	headers, err := CreateLevel2Headers(c.signer, c.creds, requestArgs)
	if err != nil {
		return nil, err
	}

	var results []interface{}
	for nextCursor != EndCursor {
		url := AddQueryOpenOrdersParams(c.host+Orders, params, nextCursor)
		resp, err := c.httpClient.Get(url[len(c.host):], headers)
		if err != nil {
			return nil, err
		}

		respMap, ok := resp.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid response format")
		}

		if cursor, ok := respMap["next_cursor"].(string); ok {
			nextCursor = cursor
		} else {
			nextCursor = EndCursor
		}

		if data, ok := respMap["data"].([]interface{}); ok {
			results = append(results, data...)
		}
	}

	return results, nil
}

// GetOrder 获取单个订单
// 需要L2认证
func (c *ClobClient) GetOrder(orderID string) (interface{}, error) {
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}

	endpoint := GetOrder + orderID
	requestArgs := &RequestArgs{
		Method:      "GET",
		RequestPath: endpoint,
	}

	headers, err := CreateLevel2Headers(c.signer, c.creds, requestArgs)
	if err != nil {
		return nil, err
	}

	return c.httpClient.Get(endpoint, headers)
}

// GetTrades 获取交易历史
// 需要L2认证
func (c *ClobClient) GetTrades(params *TradeParams, nextCursor string) ([]interface{}, error) {
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}

	if nextCursor == "" {
		nextCursor = "MA=="
	}

	requestArgs := &RequestArgs{
		Method:      "GET",
		RequestPath: Trades,
	}

	headers, err := CreateLevel2Headers(c.signer, c.creds, requestArgs)
	if err != nil {
		return nil, err
	}

	var results []interface{}
	for nextCursor != EndCursor {
		url := AddQueryTradeParams(c.host+Trades, params, nextCursor)
		resp, err := c.httpClient.Get(url[len(c.host):], headers)
		if err != nil {
			return nil, err
		}

		respMap, ok := resp.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid response format")
		}

		if cursor, ok := respMap["next_cursor"].(string); ok {
			nextCursor = cursor
		} else {
			nextCursor = EndCursor
		}

		if data, ok := respMap["data"].([]interface{}); ok {
			results = append(results, data...)
		}
	}

	return results, nil
}

// GetBalanceAllowance 获取余额和授权
// 需要L2认证
//
// 对齐 py-clob-client-v2:signature_type 参数总是从 client.sigType 取,
// 调用方传入的 params.SignatureType 若未设置则按客户端 sigType 填。
func (c *ClobClient) GetBalanceAllowance(params *BalanceAllowanceParams) (map[string]interface{}, error) {
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
	requestArgs := &RequestArgs{
		Method:      "GET",
		RequestPath: GetBalanceAllowance,
	}

	headers, err := CreateLevel2Headers(c.signer, c.creds, requestArgs)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Get(url[len(c.host):], headers)
	if err != nil {
		return nil, err
	}

	respMap, ok := resp.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}

	return respMap, nil
}

// GetNotifications 获取通知
// 需要L2认证
//
// 对齐 py-clob-client-v2:signature_type query 参数从客户端 sigType 取
// (Proxy/Safe/POLY_1271 用户会被服务端按对应钱包过滤通知)。
func (c *ClobClient) GetNotifications() (interface{}, error) {
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s?signature_type=%d", GetNotifications, c.sigType)
	requestArgs := &RequestArgs{
		Method:      "GET",
		RequestPath: GetNotifications,
	}

	headers, err := CreateLevel2Headers(c.signer, c.creds, requestArgs)
	if err != nil {
		return nil, err
	}

	return c.httpClient.Get(url, headers)
}

// DropNotifications 删除通知
// 需要L2认证
func (c *ClobClient) DropNotifications(params *DropNotificationParams) (interface{}, error) {
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}

	url := DropNotificationsQueryParams(c.host+DropNotifications, params)
	requestArgs := &RequestArgs{
		Method:      "DELETE",
		RequestPath: DropNotifications,
	}

	headers, err := CreateLevel2Headers(c.signer, c.creds, requestArgs)
	if err != nil {
		return nil, err
	}

	return c.httpClient.Delete(url[len(c.host):], headers, nil)
}

