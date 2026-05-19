package polymarket

import (
	"fmt"
	"net/url"
	"strconv"
)

// ============================================================
//  Rewards / Rebates 客户端方法
//
//  对应 Polymarket V2 文档 /api-reference/rewards/* 与 /api-reference/rebates/*
//  L2 端点用 builder 或 user API key 都行(只看 maker_address 的归属)。
//  公共端点(markets/current, /rebates/current)不需要认证。
// ============================================================

// RewardsUserQuery 是 GET /rewards/user 与 /rewards/user/total 共用的查询参数。
//
//	Date          : YYYY-MM-DD;必填。
//	SignatureType : 可选;0=EOA, 1=POLY_PROXY, 2=POLY_GNOSIS_SAFE。
//	MakerAddress  : 可选;指定要查的 maker 地址。零值时用本机 API key 关联地址。
//	Sponsored     : 可选;true 时只看 sponsored 部分。
//	NextCursor    : 可选;分页 token,从上次响应里拿。
type RewardsUserQuery struct {
	Date          string
	SignatureType *int
	MakerAddress  string
	Sponsored     *bool
	NextCursor    string
}

func (q *RewardsUserQuery) encode() string {
	if q == nil {
		return ""
	}
	v := url.Values{}
	if q.Date != "" {
		v.Set("date", q.Date)
	}
	if q.SignatureType != nil {
		v.Set("signature_type", strconv.Itoa(*q.SignatureType))
	}
	if q.MakerAddress != "" {
		v.Set("maker_address", q.MakerAddress)
	}
	if q.Sponsored != nil {
		v.Set("sponsored", strconv.FormatBool(*q.Sponsored))
	}
	if q.NextCursor != "" {
		v.Set("next_cursor", q.NextCursor)
	}
	enc := v.Encode()
	if enc == "" {
		return ""
	}
	return "?" + enc
}

// GetEarningsForUserForDay 拉单日每条 condition 的明细 earnings。需要 L2。
//
// GET /rewards/user?date=YYYY-MM-DD[&signature_type=…&maker_address=…&sponsored=…&next_cursor=…]
//
// 返回结构(分页):
//
//	{ limit, count, next_cursor, data: [{ date, condition_id, asset_address,
//	  maker_address, earnings, asset_rate }] }
func (c *ClobClient) GetEarningsForUserForDay(q *RewardsUserQuery) (interface{}, error) {
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}
	if q == nil || q.Date == "" {
		return nil, fmt.Errorf("date is required (YYYY-MM-DD)")
	}
	path := GetEarningsForUserForDay + q.encode()
	headers, err := CreateLevel2Headers(c.signer, c.creds, &RequestArgs{Method: "GET", RequestPath: GetEarningsForUserForDay})
	if err != nil {
		return nil, err
	}
	return c.httpClient.Get(path, headers)
}

// GetTotalEarningsForUserForDay 拉单日按 asset 聚合后的 earnings。需要 L2。
//
// GET /rewards/user/total?date=YYYY-MM-DD
//
// 返回数组 [{ date, asset_address, maker_address, earnings, asset_rate }]。
func (c *ClobClient) GetTotalEarningsForUserForDay(q *RewardsUserQuery) (interface{}, error) {
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}
	if q == nil || q.Date == "" {
		return nil, fmt.Errorf("date is required (YYYY-MM-DD)")
	}
	// next_cursor 在此端点 doc 上未列出,但保留也无副作用 —— 服务端会忽略。
	path := GetTotalEarningsForUserForDay + q.encode()
	headers, err := CreateLevel2Headers(c.signer, c.creds, &RequestArgs{Method: "GET", RequestPath: GetTotalEarningsForUserForDay})
	if err != nil {
		return nil, err
	}
	return c.httpClient.Get(path, headers)
}

// GetLiquidityRewardPercentages 拉每个 condition 当前给我 maker 的 reward 百分比。需要 L2。
//
// GET /rewards/user/percentages[?signature_type=…&maker_address=…]
//
// 返回 { condition_id: percentage (0-100) } 的 map。
func (c *ClobClient) GetLiquidityRewardPercentages(sigType *int, makerAddress string) (interface{}, error) {
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}
	v := url.Values{}
	if sigType != nil {
		v.Set("signature_type", strconv.Itoa(*sigType))
	}
	if makerAddress != "" {
		v.Set("maker_address", makerAddress)
	}
	path := GetLiquidityRewardPercentages
	if enc := v.Encode(); enc != "" {
		path += "?" + enc
	}
	headers, err := CreateLevel2Headers(c.signer, c.creds, &RequestArgs{Method: "GET", RequestPath: GetLiquidityRewardPercentages})
	if err != nil {
		return nil, err
	}
	return c.httpClient.Get(path, headers)
}

// GetRewardsMarketsCurrent 列当前活跃的 rewards markets 配置(公开,不需要认证)。
//
// GET /rewards/markets/current[?sponsored=…&next_cursor=…]
//
// 返回分页 [{ condition_id, rewards_max_spread, rewards_min_size,
//
//	rewards_config: [...], sponsored_daily_rate, sponsors_count,
//	native_daily_rate, total_daily_rate }]
func (c *ClobClient) GetRewardsMarketsCurrent(sponsored *bool, nextCursor string) (interface{}, error) {
	v := url.Values{}
	if sponsored != nil {
		v.Set("sponsored", strconv.FormatBool(*sponsored))
	}
	if nextCursor != "" {
		v.Set("next_cursor", nextCursor)
	}
	path := GetRewardsMarketsCurrent
	if enc := v.Encode(); enc != "" {
		path += "?" + enc
	}
	return c.httpClient.Get(path, nil)
}

// RewardsUserMarketsQuery 给 GET /rewards/user/markets 用。比 RewardsUserQuery 多很多过滤项。
type RewardsUserMarketsQuery struct {
	Date              string
	SignatureType     *int
	MakerAddress      string
	Sponsored         *bool
	NextCursor        string
	PageSize          *int // 最大 500
	Q                 string
	TagSlug           string
	FavoriteMarkets   *bool
	NoCompetition     *bool
	OnlyMergeable     *bool
	OnlyOpenOrders    *bool
	OnlyOpenPositions *bool
	OrderBy           string // max_spread/min_size/end_date/earning_percentage/rate_per_day/earnings/spread/competitiveness/question/price/market/volume_24hr
	Position          string // ASC / DESC
}

func (q *RewardsUserMarketsQuery) encode() string {
	if q == nil {
		return ""
	}
	v := url.Values{}
	if q.Date != "" {
		v.Set("date", q.Date)
	}
	if q.SignatureType != nil {
		v.Set("signature_type", strconv.Itoa(*q.SignatureType))
	}
	if q.MakerAddress != "" {
		v.Set("maker_address", q.MakerAddress)
	}
	if q.Sponsored != nil {
		v.Set("sponsored", strconv.FormatBool(*q.Sponsored))
	}
	if q.NextCursor != "" {
		v.Set("next_cursor", q.NextCursor)
	}
	if q.PageSize != nil {
		v.Set("page_size", strconv.Itoa(*q.PageSize))
	}
	if q.Q != "" {
		v.Set("q", q.Q)
	}
	if q.TagSlug != "" {
		v.Set("tag_slug", q.TagSlug)
	}
	if q.FavoriteMarkets != nil {
		v.Set("favorite_markets", strconv.FormatBool(*q.FavoriteMarkets))
	}
	if q.NoCompetition != nil {
		v.Set("no_competition", strconv.FormatBool(*q.NoCompetition))
	}
	if q.OnlyMergeable != nil {
		v.Set("only_mergeable", strconv.FormatBool(*q.OnlyMergeable))
	}
	if q.OnlyOpenOrders != nil {
		v.Set("only_open_orders", strconv.FormatBool(*q.OnlyOpenOrders))
	}
	if q.OnlyOpenPositions != nil {
		v.Set("only_open_positions", strconv.FormatBool(*q.OnlyOpenPositions))
	}
	if q.OrderBy != "" {
		v.Set("order_by", q.OrderBy)
	}
	if q.Position != "" {
		v.Set("position", q.Position)
	}
	enc := v.Encode()
	if enc == "" {
		return ""
	}
	return "?" + enc
}

// GetRewardsMarketsForUser 列 user 当前所有有 reward 资格的 markets + 我的 earnings。需要 L2。
//
// GET /rewards/user/markets[?date=&page_size=&order_by=&…]
//
// 这是用户面板"我的 reward 收益"页面背后的端点,字段非常丰富。
func (c *ClobClient) GetRewardsMarketsForUser(q *RewardsUserMarketsQuery) (interface{}, error) {
	if err := c.assertLevel2Auth(); err != nil {
		return nil, err
	}
	path := GetRewardsEarningsPercentages + q.encode()
	headers, err := CreateLevel2Headers(c.signer, c.creds, &RequestArgs{Method: "GET", RequestPath: GetRewardsEarningsPercentages})
	if err != nil {
		return nil, err
	}
	return c.httpClient.Get(path, headers)
}

// GetRewardsMarketsForCondition 查单个 condition 的 reward 配置(公开,不需要认证)。
//
// GET /rewards/markets/{condition_id}[?sponsored=…&next_cursor=…]
//
// 返回 { limit, count, next_cursor, data: [{
//
//	condition_id, question, market_slug, event_slug, image,
//	rewards_max_spread, rewards_min_size, market_competitiveness,
//	tokens: [{token_id, outcome, price}],
//	rewards_config: [{id, asset_address, start_date, end_date, rate_per_day,
//	                 total_rewards, remaining_reward_amount, total_days}]
//
// }]}
//
// 注意:虽然单 condition 但是 data 是数组(服务端做分页),通常只有 1 条。
func (c *ClobClient) GetRewardsMarketsForCondition(conditionID string, sponsored *bool, nextCursor string) (interface{}, error) {
	if conditionID == "" {
		return nil, fmt.Errorf("conditionID required")
	}
	v := url.Values{}
	if sponsored != nil {
		v.Set("sponsored", strconv.FormatBool(*sponsored))
	}
	if nextCursor != "" {
		v.Set("next_cursor", nextCursor)
	}
	path := GetRewardsMarketsEndpoint + conditionID
	if enc := v.Encode(); enc != "" {
		path += "?" + enc
	}
	return c.httpClient.Get(path, nil)
}

// GetCurrentMakerRebate 拉指定日期某 maker 的 rebate 明细。不需要认证。
//
// GET /rebates/current?date=YYYY-MM-DD&maker_address=0x…
//
// 返回数组 [{ date, condition_id, asset_address, maker_address, rebated_fees_usdc }]。
// rebated_fees_usdc 是字符串(服务端文档给的是 string,通常是 decimal 形式)。
func (c *ClobClient) GetCurrentMakerRebate(date, makerAddress string) (interface{}, error) {
	if date == "" || makerAddress == "" {
		return nil, fmt.Errorf("date and maker_address are required")
	}
	v := url.Values{}
	v.Set("date", date)
	v.Set("maker_address", makerAddress)
	path := GetCurrentMakerRebate + "?" + v.Encode()
	return c.httpClient.Get(path, nil)
}
