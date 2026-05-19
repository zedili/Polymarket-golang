// Data API client(用户头寸 / 活动 / open-interest / live-volume 等)
//
// data-api.polymarket.com 是独立服务,**全部公开,无需认证**。
// 端点对照文档:
//   - GET /positions       https://docs.polymarket.com/api-reference/core/get-current-positions-for-a-user
//   - GET /value           https://docs.polymarket.com/api-reference/core/get-total-value-of-a-users-positions
//   - GET /activity        https://docs.polymarket.com/api-reference/core/get-user-activity
//   - GET /holders         https://docs.polymarket.com/api-reference/core/get-top-holders-for-markets
//   - GET /oi              https://docs.polymarket.com/api-reference/misc/get-open-interest
//   - GET /live-volume     https://docs.polymarket.com/api-reference/misc/get-live-volume-for-an-event
//   - GET /traded          https://docs.polymarket.com/api-reference/misc/get-total-markets-a-user-has-traded
//
// 设计要点:
//   - 字段名严格对齐服务端返回(camelCase)
//   - 复杂过滤通过 *Query struct 传,简单 endpoint 直接函数参数
//   - 用 SDK 自带 httpClient 享受连接池 / 重试,但 baseURL 独立
package polymarket

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DataAPIBaseURL Polymarket Data API host.
const DataAPIBaseURL = "https://data-api.polymarket.com"

const (
	DataAPIPathPositions  = "/positions"
	DataAPIPathValue      = "/value"
	DataAPIPathActivity   = "/activity"
	DataAPIPathHolders    = "/holders"
	DataAPIPathOI         = "/oi"
	DataAPIPathLiveVolume = "/live-volume"
	DataAPIPathTraded     = "/traded"
)

// ============================================================
//  Typed response structs
// ============================================================

// Position 单一 user 头寸。对齐 /positions 返回字段。
type Position struct {
	ProxyWallet        string  `json:"proxyWallet"`
	Asset              string  `json:"asset"` // ERC1155 positionId
	ConditionID        string  `json:"conditionId"`
	Size               float64 `json:"size"`
	AvgPrice           float64 `json:"avgPrice"`
	InitialValue       float64 `json:"initialValue"`
	CurrentValue       float64 `json:"currentValue"`
	CashPnl            float64 `json:"cashPnl"`
	PercentPnl         float64 `json:"percentPnl"`
	TotalBought        float64 `json:"totalBought"`
	RealizedPnl        float64 `json:"realizedPnl"`
	PercentRealizedPnl float64 `json:"percentRealizedPnl"`
	CurPrice           float64 `json:"curPrice"`
	Redeemable         bool    `json:"redeemable"`
	Mergeable          bool    `json:"mergeable"`
	Title              string  `json:"title"`
	Slug               string  `json:"slug"`
	Icon               string  `json:"icon"`
	EventSlug          string  `json:"eventSlug"`
	Outcome            string  `json:"outcome"`
	OutcomeIndex       int     `json:"outcomeIndex"`
	OppositeOutcome    string  `json:"oppositeOutcome"`
	OppositeAsset      string  `json:"oppositeAsset"`
	EndDate            string  `json:"endDate"`
	NegativeRisk       bool    `json:"negativeRisk"`
}

// PositionsQuery /positions 查询过滤器。
// 至少要 User。Market/EventID 互斥。
type PositionsQuery struct {
	User          string   // required
	Market        []string // condition_ids
	EventID       []int    // event ids
	SizeThreshold *float64 // default 1
	Redeemable    *bool
	Mergeable     *bool
	Limit         *int    // 0-500, default 100
	Offset        *int    // 0-10000
	SortBy        string  // CURRENT/INITIAL/TOKENS/CASHPNL/PERCENTPNL/TITLE/RESOLVING/PRICE/AVGPRICE
	SortDirection string  // ASC / DESC
	Title         string  // max 100 chars
}

func (q *PositionsQuery) encode() (string, error) {
	if q == nil || q.User == "" {
		return "", fmt.Errorf("user required")
	}
	if len(q.Market) > 0 && len(q.EventID) > 0 {
		return "", fmt.Errorf("market and eventId are mutually exclusive")
	}
	v := url.Values{}
	v.Set("user", q.User)
	for _, m := range q.Market {
		v.Add("market", m)
	}
	for _, e := range q.EventID {
		v.Add("eventId", strconv.Itoa(e))
	}
	if q.SizeThreshold != nil {
		v.Set("sizeThreshold", strconv.FormatFloat(*q.SizeThreshold, 'f', -1, 64))
	}
	if q.Redeemable != nil {
		v.Set("redeemable", strconv.FormatBool(*q.Redeemable))
	}
	if q.Mergeable != nil {
		v.Set("mergeable", strconv.FormatBool(*q.Mergeable))
	}
	if q.Limit != nil {
		v.Set("limit", strconv.Itoa(*q.Limit))
	}
	if q.Offset != nil {
		v.Set("offset", strconv.Itoa(*q.Offset))
	}
	if q.SortBy != "" {
		v.Set("sortBy", q.SortBy)
	}
	if q.SortDirection != "" {
		v.Set("sortDirection", q.SortDirection)
	}
	if q.Title != "" {
		v.Set("title", q.Title)
	}
	return v.Encode(), nil
}

// ValueEntry /value 返回数组的单元素。
type ValueEntry struct {
	User  string  `json:"user"`
	Value float64 `json:"value"`
}

// ActivityType 是 /activity 的 type 枚举。
type ActivityType string

const (
	ActivityTrade           ActivityType = "TRADE"
	ActivitySplit           ActivityType = "SPLIT"
	ActivityMerge           ActivityType = "MERGE"
	ActivityRedeem          ActivityType = "REDEEM"
	ActivityReward          ActivityType = "REWARD"
	ActivityConversion      ActivityType = "CONVERSION"
	ActivityMakerRebate     ActivityType = "MAKER_REBATE"
	ActivityReferralReward  ActivityType = "REFERRAL_REWARD"
)

// ActivityQuery /activity 过滤器。
type ActivityQuery struct {
	User          string // required
	Limit         *int   // 0-500, default 100
	Offset        *int   // 0-10000, default 0
	Market        []string
	EventID       []int
	Type          []ActivityType
	Start         *int64 // unix sec
	End           *int64
	SortBy        string // TIMESTAMP / TOKENS / CASH
	SortDirection string // ASC / DESC
	Side          string // BUY / SELL
}

func (q *ActivityQuery) encode() (string, error) {
	if q == nil || q.User == "" {
		return "", fmt.Errorf("user required")
	}
	v := url.Values{}
	v.Set("user", q.User)
	if q.Limit != nil {
		v.Set("limit", strconv.Itoa(*q.Limit))
	}
	if q.Offset != nil {
		v.Set("offset", strconv.Itoa(*q.Offset))
	}
	if len(q.Market) > 0 {
		v.Set("market", strings.Join(q.Market, ","))
	}
	if len(q.EventID) > 0 {
		ss := make([]string, len(q.EventID))
		for i, e := range q.EventID {
			ss[i] = strconv.Itoa(e)
		}
		v.Set("eventId", strings.Join(ss, ","))
	}
	if len(q.Type) > 0 {
		ss := make([]string, len(q.Type))
		for i, t := range q.Type {
			ss[i] = string(t)
		}
		v.Set("type", strings.Join(ss, ","))
	}
	if q.Start != nil {
		v.Set("start", strconv.FormatInt(*q.Start, 10))
	}
	if q.End != nil {
		v.Set("end", strconv.FormatInt(*q.End, 10))
	}
	if q.SortBy != "" {
		v.Set("sortBy", q.SortBy)
	}
	if q.SortDirection != "" {
		v.Set("sortDirection", q.SortDirection)
	}
	if q.Side != "" {
		v.Set("side", q.Side)
	}
	return v.Encode(), nil
}

// HoldersQuery /holders 过滤器。
type HoldersQuery struct {
	Markets    []string // required
	Limit      *int     // default 20, max 20
	MinBalance *int     // default 1, 0-999999
}

// MetaHolder /holders 数组单元素。
type MetaHolder struct {
	Token   string   `json:"token"`
	Holders []Holder `json:"holders"`
}

type Holder struct {
	ProxyWallet  string  `json:"proxyWallet"`
	Amount       float64 `json:"amount"`
	Pseudonym    string  `json:"pseudonym"`
	ProfileImage string  `json:"profileImage"`
	OutcomeIndex int     `json:"outcomeIndex"`
}

// OpenInterest /oi 单元素。
type OpenInterest struct {
	Market string  `json:"market"`
	Value  float64 `json:"value"`
}

// LiveVolume /live-volume 单元素。
type LiveVolume struct {
	Total   float64 `json:"total"`
	Markets []struct {
		Market string  `json:"market"`
		Value  float64 `json:"value"`
	} `json:"markets"`
}

// TradedSummary /traded 响应。
type TradedSummary struct {
	User   string `json:"user"`
	Traded int    `json:"traded"`
}

// ============================================================
//  Client
// ============================================================

// DataAPIClient Polymarket Data API client。
type DataAPIClient struct {
	baseURL string
	http    *http.Client
}

// NewDataAPIClient 创建默认 client。
func NewDataAPIClient() *DataAPIClient {
	return &DataAPIClient{
		baseURL: DataAPIBaseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// WithBaseURL 测试用 URL 注入。
func (c *DataAPIClient) WithBaseURL(u string) *DataAPIClient { c.baseURL = u; return c }

// WithHTTPClient 注入自定义 http client。
func (c *DataAPIClient) WithHTTPClient(h *http.Client) *DataAPIClient { c.http = h; return c }

// ============================================================
//  方法
// ============================================================

// GetPositions 拉某 user 当前所有头寸,按 PositionsQuery 过滤。
func (c *DataAPIClient) GetPositions(q *PositionsQuery) ([]Position, error) {
	qs, err := q.encode()
	if err != nil {
		return nil, err
	}
	var out []Position
	if err := c.get(DataAPIPathPositions+"?"+qs, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetTotalValue 拉某 user 所有头寸总市值(可选按 market 过滤)。
// 返回数组只有 1 个元素(per-user),保留数组形态对齐服务端。
func (c *DataAPIClient) GetTotalValue(user string, markets []string) ([]ValueEntry, error) {
	if user == "" {
		return nil, fmt.Errorf("user required")
	}
	v := url.Values{}
	v.Set("user", user)
	for _, m := range markets {
		v.Add("market", m)
	}
	var out []ValueEntry
	if err := c.get(DataAPIPathValue+"?"+v.Encode(), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetActivity 拉某 user 的活动流(下单/成交/赎回/分裂/合并/reward/rebate/referral)。
// 返回 interface{} 因为单条记录字段随 ActivityType 变,调用方自行类型断言。
func (c *DataAPIClient) GetActivity(q *ActivityQuery) (interface{}, error) {
	qs, err := q.encode()
	if err != nil {
		return nil, err
	}
	var out interface{}
	if err := c.get(DataAPIPathActivity+"?"+qs, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetHolders 拉若干个 condition 的 top holders。
func (c *DataAPIClient) GetHolders(q *HoldersQuery) ([]MetaHolder, error) {
	if q == nil || len(q.Markets) == 0 {
		return nil, fmt.Errorf("markets required")
	}
	v := url.Values{}
	v.Set("market", strings.Join(q.Markets, ","))
	if q.Limit != nil {
		v.Set("limit", strconv.Itoa(*q.Limit))
	}
	if q.MinBalance != nil {
		v.Set("minBalance", strconv.Itoa(*q.MinBalance))
	}
	var out []MetaHolder
	if err := c.get(DataAPIPathHolders+"?"+v.Encode(), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetOpenInterest 拉若干 condition 的 open interest 总额。
func (c *DataAPIClient) GetOpenInterest(markets []string) ([]OpenInterest, error) {
	if len(markets) == 0 {
		return nil, fmt.Errorf("markets required")
	}
	v := url.Values{}
	for _, m := range markets {
		v.Add("market", m)
	}
	var out []OpenInterest
	if err := c.get(DataAPIPathOI+"?"+v.Encode(), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetLiveVolume 拉某 event 各 market 的实时成交额。
func (c *DataAPIClient) GetLiveVolume(eventID int) ([]LiveVolume, error) {
	if eventID < 1 {
		return nil, fmt.Errorf("eventID must be >= 1")
	}
	v := url.Values{}
	v.Set("id", strconv.Itoa(eventID))
	var out []LiveVolume
	if err := c.get(DataAPIPathLiveVolume+"?"+v.Encode(), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetTotalMarketsTraded 拉某 user 历史成交过多少个 market。
func (c *DataAPIClient) GetTotalMarketsTraded(user string) (*TradedSummary, error) {
	if user == "" {
		return nil, fmt.Errorf("user required")
	}
	v := url.Values{}
	v.Set("user", user)
	var out TradedSummary
	if err := c.get(DataAPIPathTraded+"?"+v.Encode(), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ============================================================
//  内部
// ============================================================

func (c *DataAPIClient) get(path string, out interface{}) error {
	req, err := http.NewRequest("GET", c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("data-api GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		var ev struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &ev)
		if ev.Error != "" {
			return fmt.Errorf("data-api GET %s -> %d: %s", path, resp.StatusCode, ev.Error)
		}
		return fmt.Errorf("data-api GET %s -> %d: %s", path, resp.StatusCode, string(data))
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode data-api response: %w (body=%s)", err, string(data))
	}
	return nil
}
