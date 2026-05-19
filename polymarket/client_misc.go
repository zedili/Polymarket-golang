package polymarket

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// CreateReadonlyAPIKey 创建只读API密钥
// 需要L2认证
func (c *ClobClient) CreateReadonlyAPIKey() (*ReadonlyApiKeyResponse, error) {
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}

	requestArgs := &RequestArgs{
		Method:      "POST",
		RequestPath: CreateReadonlyAPIKey,
	}

	headers, err := CreateLevel2Headers(c.signer, c.creds, requestArgs)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Post(CreateReadonlyAPIKey, headers, nil)
	if err != nil {
		return nil, err
	}

	respMap, ok := resp.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}

	return &ReadonlyApiKeyResponse{
		APIKey: getStringFromMap(respMap, "apiKey"),
	}, nil
}

// GetReadonlyAPIKeys 获取只读API密钥列表
// 需要L2认证
func (c *ClobClient) GetReadonlyAPIKeys() (interface{}, error) {
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}

	requestArgs := &RequestArgs{
		Method:      "GET",
		RequestPath: GetReadonlyAPIKeys,
	}

	headers, err := CreateLevel2Headers(c.signer, c.creds, requestArgs)
	if err != nil {
		return nil, err
	}

	return c.httpClient.Get(GetReadonlyAPIKeys, headers)
}

// DeleteReadonlyAPIKey 删除只读API密钥
// 需要L2认证
func (c *ClobClient) DeleteReadonlyAPIKey(key string) (interface{}, error) {
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}

	body := map[string]string{"key": key}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	bodyStr := string(bodyJSON)

	requestArgs := &RequestArgs{
		Method:         "DELETE",
		RequestPath:    DeleteReadonlyAPIKey,
		Body:           body,
		SerializedBody: &bodyStr,
	}

	headers, err := CreateLevel2Headers(c.signer, c.creds, requestArgs)
	if err != nil {
		return nil, err
	}

	return c.httpClient.Delete(DeleteReadonlyAPIKey, headers, bodyStr)
}

// ValidateReadonlyAPIKey 验证只读API密钥
// 公开端点，不需要认证
func (c *ClobClient) ValidateReadonlyAPIKey(address, key string) (interface{}, error) {
	path := fmt.Sprintf("%s?address=%s&key=%s", ValidateReadonlyAPIKey, address, key)
	return c.httpClient.Get(path, nil)
}

// IsOrderScoring 检查订单是否正在评分
// 需要L2认证
func (c *ClobClient) IsOrderScoring(params *OrderScoringParams) (interface{}, error) {
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}

	url := AddOrderScoringParamsToURL(c.host+IsOrderScoring, params)
	requestArgs := &RequestArgs{
		Method:      "GET",
		RequestPath: IsOrderScoring,
	}

	headers, err := CreateLevel2Headers(c.signer, c.creds, requestArgs)
	if err != nil {
		return nil, err
	}

	return c.httpClient.Get(url[len(c.host):], headers)
}

// AreOrdersScoring 检查多个订单是否正在评分
// 需要L2认证
func (c *ClobClient) AreOrdersScoring(params *OrdersScoringParams) (interface{}, error) {
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}

	bodyJSON, err := json.Marshal(params.OrderIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal order IDs: %w", err)
	}
	bodyStr := string(bodyJSON)

	requestArgs := &RequestArgs{
		Method:         "POST",
		RequestPath:    AreOrdersScoring,
		Body:           params.OrderIDs,
		SerializedBody: &bodyStr,
	}

	headers, err := CreateLevel2Headers(c.signer, c.creds, requestArgs)
	if err != nil {
		return nil, err
	}

	return c.httpClient.Post(AreOrdersScoring, headers, bodyStr)
}

// GetMarkets 获取市场列表
func (c *ClobClient) GetMarkets(nextCursor string) (interface{}, error) {
	if nextCursor == "" {
		nextCursor = "MA=="
	}
	path := fmt.Sprintf("%s?next_cursor=%s", GetMarkets, nextCursor)
	return c.httpClient.Get(path, nil)
}

// GetSimplifiedMarkets 获取简化市场列表
func (c *ClobClient) GetSimplifiedMarkets(nextCursor string) (interface{}, error) {
	if nextCursor == "" {
		nextCursor = "MA=="
	}
	path := fmt.Sprintf("%s?next_cursor=%s", GetSimplifiedMarkets, nextCursor)
	return c.httpClient.Get(path, nil)
}

// GetSamplingMarkets 获取采样市场列表
func (c *ClobClient) GetSamplingMarkets(nextCursor string) (interface{}, error) {
	if nextCursor == "" {
		nextCursor = "MA=="
	}
	path := fmt.Sprintf("%s?next_cursor=%s", GetSamplingMarkets, nextCursor)
	return c.httpClient.Get(path, nil)
}

// GetSamplingSimplifiedMarkets 获取采样简化市场列表
func (c *ClobClient) GetSamplingSimplifiedMarkets(nextCursor string) (interface{}, error) {
	if nextCursor == "" {
		nextCursor = "MA=="
	}
	path := fmt.Sprintf("%s?next_cursor=%s", GetSamplingSimplifiedMarkets, nextCursor)
	return c.httpClient.Get(path, nil)
}

// GetMarket 根据condition_id获取市场
func (c *ClobClient) GetMarket(conditionID string) (interface{}, error) {
	path := GetMarket + conditionID
	return c.httpClient.Get(path, nil)
}

// GetMarketTradesEvents 根据condition_id获取市场交易事件
func (c *ClobClient) GetMarketTradesEvents(conditionID string) (interface{}, error) {
	path := GetMarketTradesEvents + conditionID
	return c.httpClient.Get(path, nil)
}

// UpdateBalanceAllowance 更新余额和授权
// 需要L2认证
//
// 对齐 py-clob-client-v2:HTTP 方法是 GET(不是 POST);signature_type 从
// client.sigType 取。
func (c *ClobClient) UpdateBalanceAllowance(params *BalanceAllowanceParams) (interface{}, error) {
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

	url := AddBalanceAllowanceParamsToURL(c.host+UpdateBalanceAllowance, params)
	requestArgs := &RequestArgs{
		Method:      "GET",
		RequestPath: UpdateBalanceAllowance,
	}

	headers, err := CreateLevel2Headers(c.signer, c.creds, requestArgs)
	if err != nil {
		return nil, err
	}

	return c.httpClient.Get(url[len(c.host):], headers)
}

// GetOrderBookHash 获取订单簿哈希
func (c *ClobClient) GetOrderBookHash(orderbook *OrderBookSummary) string {
	return GenerateOrderBookSummaryHash(orderbook)
}

// GetBuilderTrades 获取Builder交易记录
// GetBuilderTrades 查询 builder 归因下的成交记录(V2)。
//
// 对齐 py-clob-client-v2.get_builder_trades:**无需 L2 auth**,服务端通过
// query 参数中的 builder_code 做归因。builder_code 不能为空 / Bytes32Zero。
//
// 参数 params:
//   - BuilderCode(必填,bytes32 hex)
//   - 可选过滤:Market / AssetID / MakerAddress / Before / After / ID
func (c *ClobClient) GetBuilderTrades(params *BuilderTradeParams, nextCursor string) ([]interface{}, error) {
	if params == nil || params.BuilderCode == "" || params.BuilderCode == Bytes32Zero {
		return nil, fmt.Errorf("builder_code is required and cannot be zero")
	}
	if nextCursor == "" {
		nextCursor = "MA=="
	}
	var results []interface{}
	for nextCursor != EndCursor {
		u := addBuilderTradesQuery(c.host+GetBuilderTrades, params, nextCursor)
		resp, err := c.httpClient.Get(u[len(c.host):], nil) // 无 auth
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

// BuilderTradeParams /builder/trades 查询参数。
type BuilderTradeParams struct {
	BuilderCode  string // 必填,bytes32 hex(必须非 zero)
	ID           string
	MakerAddress string
	Market       string
	AssetID      string
	Before       int64
	After        int64
}

// addBuilderTradesQuery 把 BuilderTradeParams 编码为 query string。
// 用 url.Values 保证所有过滤字段(market / asset_id / maker_address / id 等)
// 即使包含 & ? = 空格 #(虽然实际是 hex 不会出现)也能被正确编码。
func addBuilderTradesQuery(base string, p *BuilderTradeParams, cursor string) string {
	q := url.Values{}
	q.Set("builder_code", p.BuilderCode)
	if p.ID != "" {
		q.Set("id", p.ID)
	}
	if p.MakerAddress != "" {
		q.Set("maker_address", p.MakerAddress)
	}
	if p.Market != "" {
		q.Set("market", p.Market)
	}
	if p.AssetID != "" {
		q.Set("asset_id", p.AssetID)
	}
	if p.Before > 0 {
		q.Set("before", strconv.FormatInt(p.Before, 10))
	}
	if p.After > 0 {
		q.Set("after", strconv.FormatInt(p.After, 10))
	}
	q.Set("next_cursor", cursor)

	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return base + sep + q.Encode()
}

// PostHeartbeat 发送心跳
// 如果心跳启动后10秒内没有发送心跳，所有订单将被取消
// 需要L2认证
func (c *ClobClient) PostHeartbeat(heartbeatID *string) (interface{}, error) {
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}

	// 对齐 py-clob-client-v2.post_heartbeat: body = {"heartbeat_id": "..."}。
	// Python 默认空字符串而不是 null。
	id := ""
	if heartbeatID != nil {
		id = *heartbeatID
	}
	body := map[string]interface{}{"heartbeat_id": id}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal heartbeat: %w", err)
	}
	bodyStr := string(bodyJSON)

	requestArgs := &RequestArgs{
		Method:         "POST",
		RequestPath:    PostHeartbeat,
		Body:           body,
		SerializedBody: &bodyStr,
	}

	headers, err := CreateLevel2Headers(c.signer, c.creds, requestArgs)
	if err != nil {
		return nil, err
	}

	return c.httpClient.Post(PostHeartbeat, headers, bodyStr)
}
