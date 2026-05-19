// Gamma API client(公开市场 / 事件 / 系列 / 标签元数据)
//
// gamma-api.polymarket.com 是公开元数据服务,**全部无需认证**。
// 端点对照:
//   - GET /markets               https://docs.polymarket.com/api-reference/markets/list-markets
//   - GET /markets/{id}          https://docs.polymarket.com/api-reference/markets/get-market-by-id
//   - GET /markets/slug/{slug}   https://docs.polymarket.com/api-reference/markets/get-market-by-slug
//   - GET /events                https://docs.polymarket.com/api-reference/events/list-events
//   - GET /events/{id}           https://docs.polymarket.com/api-reference/events/get-event-by-id
//   - GET /events/slug/{slug}    https://docs.polymarket.com/api-reference/events/get-event-by-slug
//
// 字段非常多(Market 150+ 字段、Event 几十),所以返回类型默认是 json.RawMessage
// 或 interface{},调用方按需 Unmarshal 到自己关心的子集。这样 schema 升级不会
// 让 SDK 频繁出现 breaking change。
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

// GammaAPIBaseURL Polymarket gamma host.
const GammaAPIBaseURL = "https://gamma-api.polymarket.com"

const (
	GammaPathMarkets     = "/markets"
	GammaPathMarketsSlug = "/markets/slug/" // + {slug}
	GammaPathEvents      = "/events"
	GammaPathEventsSlug  = "/events/slug/" // + {slug}
)

// ============================================================
//  Query filters
// ============================================================

// GammaMarketsQuery /markets 过滤器。所有字段可选,服务端按缺省策略返回。
type GammaMarketsQuery struct {
	Limit              *int
	Offset             *int
	Order              string   // comma-separated field names
	Ascending          *bool
	IDs                []int    // ?id=...
	Slugs              []string // ?slug=...
	ClobTokenIDs       []string // ?clob_token_ids=...
	ConditionIDs       []string // ?condition_ids=...
	MarketMakerAddrs   []string // ?market_maker_address=...
	LiquidityNumMin    *float64
	LiquidityNumMax    *float64
	VolumeNumMin       *float64
	VolumeNumMax       *float64
	StartDateMin       string // ISO date-time
	StartDateMax       string
	EndDateMin         string
	EndDateMax         string
	TagID              *int
	RelatedTags        *bool
	CYOM               *bool
	UMAResolutionState string // string
	GameID             string
	SportsMarketTypes  []string
	RewardsMinSize     *float64
	QuestionIDs        []string
	IncludeTag         *bool
	Closed             *bool // default false
	Active             *bool // useful filter
}

func (q *GammaMarketsQuery) encode() string {
	v := url.Values{}
	if q == nil {
		return ""
	}
	if q.Limit != nil {
		v.Set("limit", strconv.Itoa(*q.Limit))
	}
	if q.Offset != nil {
		v.Set("offset", strconv.Itoa(*q.Offset))
	}
	if q.Order != "" {
		v.Set("order", q.Order)
	}
	if q.Ascending != nil {
		v.Set("ascending", strconv.FormatBool(*q.Ascending))
	}
	for _, x := range q.IDs {
		v.Add("id", strconv.Itoa(x))
	}
	for _, x := range q.Slugs {
		v.Add("slug", x)
	}
	for _, x := range q.ClobTokenIDs {
		v.Add("clob_token_ids", x)
	}
	for _, x := range q.ConditionIDs {
		v.Add("condition_ids", x)
	}
	for _, x := range q.MarketMakerAddrs {
		v.Add("market_maker_address", x)
	}
	if q.LiquidityNumMin != nil {
		v.Set("liquidity_num_min", strconv.FormatFloat(*q.LiquidityNumMin, 'f', -1, 64))
	}
	if q.LiquidityNumMax != nil {
		v.Set("liquidity_num_max", strconv.FormatFloat(*q.LiquidityNumMax, 'f', -1, 64))
	}
	if q.VolumeNumMin != nil {
		v.Set("volume_num_min", strconv.FormatFloat(*q.VolumeNumMin, 'f', -1, 64))
	}
	if q.VolumeNumMax != nil {
		v.Set("volume_num_max", strconv.FormatFloat(*q.VolumeNumMax, 'f', -1, 64))
	}
	for k, val := range map[string]string{
		"start_date_min": q.StartDateMin,
		"start_date_max": q.StartDateMax,
		"end_date_min":   q.EndDateMin,
		"end_date_max":   q.EndDateMax,
		"uma_resolution_status": q.UMAResolutionState,
		"game_id":               q.GameID,
	} {
		if val != "" {
			v.Set(k, val)
		}
	}
	if q.TagID != nil {
		v.Set("tag_id", strconv.Itoa(*q.TagID))
	}
	if q.RelatedTags != nil {
		v.Set("related_tags", strconv.FormatBool(*q.RelatedTags))
	}
	if q.CYOM != nil {
		v.Set("cyom", strconv.FormatBool(*q.CYOM))
	}
	for _, x := range q.SportsMarketTypes {
		v.Add("sports_market_types", x)
	}
	if q.RewardsMinSize != nil {
		v.Set("rewards_min_size", strconv.FormatFloat(*q.RewardsMinSize, 'f', -1, 64))
	}
	for _, x := range q.QuestionIDs {
		v.Add("question_ids", x)
	}
	if q.IncludeTag != nil {
		v.Set("include_tag", strconv.FormatBool(*q.IncludeTag))
	}
	if q.Closed != nil {
		v.Set("closed", strconv.FormatBool(*q.Closed))
	}
	if q.Active != nil {
		v.Set("active", strconv.FormatBool(*q.Active))
	}
	return v.Encode()
}

// GammaEventsQuery /events 过滤器。
type GammaEventsQuery struct {
	Limit           *int
	Offset          *int
	Order           string
	Ascending       *bool
	IDs             []int
	TagID           *int
	ExcludeTagIDs   []int
	Slugs           []string
	TagSlug         string
	RelatedTags     *bool
	Active          *bool
	Archived        *bool
	Featured        *bool
	CYOM            *bool
	IncludeChat     *bool
	IncludeTemplate *bool
	Recurrence      string
	Closed          *bool
	LiquidityMin    *float64
	LiquidityMax    *float64
	VolumeMin       *float64
	VolumeMax       *float64
	StartDateMin    string
	StartDateMax    string
	EndDateMin      string
	EndDateMax      string
}

func (q *GammaEventsQuery) encode() string {
	v := url.Values{}
	if q == nil {
		return ""
	}
	if q.Limit != nil {
		v.Set("limit", strconv.Itoa(*q.Limit))
	}
	if q.Offset != nil {
		v.Set("offset", strconv.Itoa(*q.Offset))
	}
	if q.Order != "" {
		v.Set("order", q.Order)
	}
	if q.Ascending != nil {
		v.Set("ascending", strconv.FormatBool(*q.Ascending))
	}
	for _, x := range q.IDs {
		v.Add("id", strconv.Itoa(x))
	}
	if q.TagID != nil {
		v.Set("tag_id", strconv.Itoa(*q.TagID))
	}
	for _, x := range q.ExcludeTagIDs {
		v.Add("exclude_tag_id", strconv.Itoa(x))
	}
	for _, x := range q.Slugs {
		v.Add("slug", x)
	}
	if q.TagSlug != "" {
		v.Set("tag_slug", q.TagSlug)
	}
	if q.RelatedTags != nil {
		v.Set("related_tags", strconv.FormatBool(*q.RelatedTags))
	}
	for k, val := range map[string]*bool{
		"active":           q.Active,
		"archived":         q.Archived,
		"featured":         q.Featured,
		"cyom":             q.CYOM,
		"include_chat":     q.IncludeChat,
		"include_template": q.IncludeTemplate,
		"closed":           q.Closed,
	} {
		if val != nil {
			v.Set(k, strconv.FormatBool(*val))
		}
	}
	if q.Recurrence != "" {
		v.Set("recurrence", q.Recurrence)
	}
	for k, val := range map[string]*float64{
		"liquidity_min": q.LiquidityMin,
		"liquidity_max": q.LiquidityMax,
		"volume_min":    q.VolumeMin,
		"volume_max":    q.VolumeMax,
	} {
		if val != nil {
			v.Set(k, strconv.FormatFloat(*val, 'f', -1, 64))
		}
	}
	for k, val := range map[string]string{
		"start_date_min": q.StartDateMin,
		"start_date_max": q.StartDateMax,
		"end_date_min":   q.EndDateMin,
		"end_date_max":   q.EndDateMax,
	} {
		if val != "" {
			v.Set(k, val)
		}
	}
	return v.Encode()
}

// ============================================================
//  Client
// ============================================================

// GammaAPIClient Polymarket gamma 客户端。
type GammaAPIClient struct {
	baseURL string
	http    *http.Client
}

// NewGammaAPIClient 默认 client。
func NewGammaAPIClient() *GammaAPIClient {
	return &GammaAPIClient{
		baseURL: GammaAPIBaseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *GammaAPIClient) WithBaseURL(u string) *GammaAPIClient     { c.baseURL = u; return c }
func (c *GammaAPIClient) WithHTTPClient(h *http.Client) *GammaAPIClient { c.http = h; return c }

// ============================================================
//  Markets
// ============================================================

// ListMarkets 返回 []GammaMarket(字段保留 raw)。每个 element 是 map[string]any
// 形式 —— Market schema 150+ 字段太多,SDK 不固化所有字段。
//
// 想要 typed Market 的话:
//
//	raw, _ := client.ListMarkets(&GammaMarketsQuery{Closed: ptrBool(false)})
//	var ms []YourMarketType
//	json.Unmarshal(raw, &ms)
func (c *GammaAPIClient) ListMarkets(q *GammaMarketsQuery) (json.RawMessage, error) {
	path := GammaPathMarkets
	if enc := q.encode(); enc != "" {
		path += "?" + enc
	}
	return c.getRaw(path)
}

// GetMarketByID 按 id 拿单个 Market。
func (c *GammaAPIClient) GetMarketByID(id int, includeTag *bool) (json.RawMessage, error) {
	path := fmt.Sprintf("%s/%d", GammaPathMarkets, id)
	if includeTag != nil {
		path += "?include_tag=" + strconv.FormatBool(*includeTag)
	}
	return c.getRaw(path)
}

// GetMarketBySlug 按 slug 拿单个 Market。
func (c *GammaAPIClient) GetMarketBySlug(slug string, includeTag *bool) (json.RawMessage, error) {
	if slug == "" {
		return nil, fmt.Errorf("slug required")
	}
	path := GammaPathMarketsSlug + slug
	if includeTag != nil {
		path += "?include_tag=" + strconv.FormatBool(*includeTag)
	}
	return c.getRaw(path)
}

// ============================================================
//  Events
// ============================================================

// ListEvents 列出 events。
func (c *GammaAPIClient) ListEvents(q *GammaEventsQuery) (json.RawMessage, error) {
	path := GammaPathEvents
	if enc := q.encode(); enc != "" {
		path += "?" + enc
	}
	return c.getRaw(path)
}

// GetEventByID 按 id 拿 event。include_chat / include_template 控制是否带聊天 / 模板数据。
func (c *GammaAPIClient) GetEventByID(id int, includeChat, includeTemplate *bool) (json.RawMessage, error) {
	path := fmt.Sprintf("%s/%d", GammaPathEvents, id)
	path += encodeIncludeFlags(includeChat, includeTemplate)
	return c.getRaw(path)
}

// GetEventBySlug 按 slug 拿 event。
func (c *GammaAPIClient) GetEventBySlug(slug string, includeChat, includeTemplate *bool) (json.RawMessage, error) {
	if slug == "" {
		return nil, fmt.Errorf("slug required")
	}
	path := GammaPathEventsSlug + slug
	path += encodeIncludeFlags(includeChat, includeTemplate)
	return c.getRaw(path)
}

func encodeIncludeFlags(includeChat, includeTemplate *bool) string {
	v := url.Values{}
	if includeChat != nil {
		v.Set("include_chat", strconv.FormatBool(*includeChat))
	}
	if includeTemplate != nil {
		v.Set("include_template", strconv.FormatBool(*includeTemplate))
	}
	if enc := v.Encode(); enc != "" {
		return "?" + enc
	}
	return ""
}

// ============================================================
//  内部
// ============================================================

func (c *GammaAPIClient) getRaw(path string) (json.RawMessage, error) {
	req, err := http.NewRequest("GET", c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gamma-api GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		// gamma 错误体常常是 string 或 {"error":"..."}
		var ev struct{ Error string `json:"error"` }
		_ = json.Unmarshal(data, &ev)
		if ev.Error != "" {
			return nil, fmt.Errorf("gamma-api GET %s -> %d: %s", path, resp.StatusCode, ev.Error)
		}
		return nil, fmt.Errorf("gamma-api GET %s -> %d: %s", path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}
