package polymarket

import (
	"encoding/base64"

	"github.com/polymarket/go-order-utils/pkg/model"
)

// base64URLDecode 包装 encoding/base64 URL safe decode。
func base64URLDecode(s string) ([]byte, error) {
	return base64.URLEncoding.DecodeString(s)
}

// ApiCreds API凭证
type ApiCreds struct {
	APIKey     string `json:"apiKey"`
	APISecret  string `json:"secret"`
	APIPassphrase string `json:"passphrase"`

	// decodedSecret 缓存 base64-url-decoded 的 APISecret 字节,免去每次 HMAC
	// 签名时重复 decode。**调用方不要手动设置,SDK 会在 SetAPICreds 时填**。
	// 标签 json:"-" 表示它不会被序列化(避免泄漏到日志)。
	decodedSecret []byte `json:"-"`
}

// DecodedSecret 返回 base64 解码后的 APISecret 字节。失败时返回 (nil, err)。
// 第一次调用会做 decode 并缓存结果,后续调用直接返回 cached 值 —— 用于
// 高频 HMAC 签名(机器人场景)的 fast path。
func (a *ApiCreds) DecodedSecret() ([]byte, error) {
	if len(a.decodedSecret) > 0 {
		return a.decodedSecret, nil
	}
	b, err := base64URLDecode(a.APISecret)
	if err != nil {
		return nil, err
	}
	a.decodedSecret = b
	return b, nil
}

// ReadonlyApiKeyResponse 只读API密钥响应
type ReadonlyApiKeyResponse struct {
	APIKey string `json:"apiKey"`
}

// RequestArgs 请求参数
type RequestArgs struct {
	Method        string
	RequestPath   string
	Body          interface{}
	SerializedBody *string
}

// BookParams 订单簿参数
type BookParams struct {
	TokenID string `json:"token_id"`
	Side    string `json:"side,omitempty"`
}

// V1 订单输入类型 (OrderArgs / MarketOrderArgs) 已在 V2 迁移中删除。
// 普通下单请用 OrderArgsV2 / MarketOrderArgsV2(见 types_v2.go)。
// RFQ 内部仍构造 V1 订单,但走 unexported rfqOrderArgsV1,不公开。

// TradeParams 交易查询参数
type TradeParams struct {
	ID           string `json:"id,omitempty"`
	MakerAddress string `json:"maker_address,omitempty"`
	Market       string `json:"market,omitempty"`
	AssetID      string `json:"asset_id,omitempty"`
	Before       int    `json:"before,omitempty"`
	After        int    `json:"after,omitempty"`
}

// OpenOrderParams 开放订单查询参数
type OpenOrderParams struct {
	ID      string `json:"id,omitempty"`
	Market  string `json:"market,omitempty"`
	AssetID string `json:"asset_id,omitempty"`
}

// DropNotificationParams 删除通知参数
type DropNotificationParams struct {
	IDs []string `json:"ids,omitempty"`
}

// OrderSummary 订单摘要
type OrderSummary struct {
	Price string `json:"price"`
	Size  string `json:"size"`
}

// OrderBookSummary 订单簿摘要
type OrderBookSummary struct {
	Market        string         `json:"market"`
	AssetID       string         `json:"asset_id"`
	Timestamp     string         `json:"timestamp"`
	Bids          []OrderSummary `json:"bids"`
	Asks          []OrderSummary `json:"asks"`
	MinOrderSize  string         `json:"min_order_size"`
	NegRisk       bool           `json:"neg_risk"`
	TickSize      string         `json:"tick_size"`
	Hash          string         `json:"hash"`
}

// AssetType 资产类型
type AssetType string

const (
	AssetTypeCollateral  AssetType = "COLLATERAL"  // 抵押品（如USDC）
	AssetTypeConditional AssetType = "CONDITIONAL" // 条件代币
)

// BalanceAllowanceParams 余额和授权查询参数
type BalanceAllowanceParams struct {
	AssetType     AssetType `json:"asset_type,omitempty"`
	TokenID       string    `json:"token_id,omitempty"`
	SignatureType *int      `json:"signature_type,omitempty"` // 指针类型，允许nil表示未设置
}

// BalanceAllowanceResponse 余额和授权响应
type BalanceAllowanceResponse struct {
	Balance   string `json:"balance"`
	Allowance string `json:"allowance"`
}

// OrderScoringParams 订单评分参数
type OrderScoringParams struct {
	OrderID string `json:"order_id"`
}

// OrdersScoringParams 多个订单评分参数
type OrdersScoringParams struct {
	OrderIDs []string `json:"order_ids"`
}

// CreateOrderOptions 创建订单选项
type CreateOrderOptions struct {
	TickSize TickSize `json:"tick_size"`
	NegRisk  bool     `json:"neg_risk"`
}

// PartialCreateOrderOptions 部分创建订单选项
type PartialCreateOrderOptions struct {
	TickSize  *TickSize  `json:"tick_size,omitempty"`  // tick size（RawOrder 模式下必须提供）
	NegRisk   *bool      `json:"neg_risk,omitempty"`   // neg risk（RawOrder 模式下必须提供）
	RawOrder  bool       `json:"raw_order,omitempty"`  // 跳过从服务器获取 tick_size/neg_risk/fee_rate，必须提供 TickSize 和 NegRisk
	OrderType *OrderType `json:"order_type,omitempty"` // 订单类型：GTC, FOK, GTD, FAK（默认 GTC）
}

// RoundConfig 舍入配置
type RoundConfig struct {
	Price  int // 价格小数位数
	Size   int // 数量小数位数
	Amount int // 金额小数位数
}

// ContractConfig 合约配置
// 同时包含 V1 和 V2 的合约地址 —— 以及 USDC、CTF 等公共依赖。
//
// 历史上 Go SDK 只暴露一个 Exchange 字段(根据 negRisk 在 V1 标准/V1 negRisk
// 之间切换)。V2 上线后链上同时存在两组 Exchange 合约,所以本结构改为同时持有
// 全部地址,具体使用哪个由调用方按 (orderVersion, negRisk) 自行选择。
type ContractConfig struct {
	// V1 (legacy) 交易所
	Exchange         string `json:"exchange"`             // V1 CTFExchange
	NegRiskExchange  string `json:"neg_risk_exchange"`    // V1 NegRiskCTFExchange

	// V2 交易所(对应 py-clob-client-v2)
	ExchangeV2        string `json:"exchange_v2"`          // V2 CTFExchange
	NegRiskExchangeV2 string `json:"neg_risk_exchange_v2"` // V2 NegRiskCTFExchange

	// 公共依赖
	Collateral        string `json:"collateral"`           // ERC20 抵押品 (USDC)
	ConditionalTokens string `json:"conditional_tokens"`   // ERC1155 条件代币
	NegRiskAdapter    string `json:"neg_risk_adapter"`     // NegRisk 适配器
}

// GetExchangeForVersion 根据订单版本和 negRisk 标志选择交易所地址。
// version 取值 1 / 2;version=2 是 V2 默认。
func (c *ContractConfig) GetExchangeForVersion(version int, negRisk bool) string {
	if version == 2 {
		if negRisk {
			return c.NegRiskExchangeV2
		}
		return c.ExchangeV2
	}
	if negRisk {
		return c.NegRiskExchange
	}
	return c.Exchange
}

// SignedOrder 已签名的 V1 订单(包装 go-order-utils.SignedOrder)。
// V2 迁移后,普通下单不再使用此类型 —— 见 SignedOrderV2 (order_builder)。
// 保留是因为 RFQ 内部路径仍需要 V1 订单格式。
type SignedOrder = model.SignedOrder

// V1 PostOrdersArgs / PostOrderResult / PostOrdersResult 已在 V2 迁移中删除。
// 批量 / 单笔下单的返回类型见 PostOrdersResultV2 / PostOrderResultV2(types_v2.go)。

