package polymarket

// 本文件定义 V2 (CTF Exchange V2) 相关的数据类型。
// 与 py-clob-client-v2/py_clob_client_v2/clob_types.py 对齐。
//
// V2 主要变化:
//   - 订单结构移除 taker / feeRateBps / nonce,新增 timestamp / metadata / builder
//   - 新增 SignatureType POLY_1271 = 3 (智能合约 Deposit Wallet)
//   - 域名版本从 "1" 升级到 "2"
//   - 新增 builder code (bytes32) 用于第三方返佣

import (
	"fmt"
	"math/big"
	"strings"

	obuilder "github.com/0xNetuser/Polymarket-golang/polymarket/order_builder"
)

// SignedOrderV2 是已签名的 V2 订单(类型别名,真实实现在 order_builder 包)。
type SignedOrderV2 = obuilder.SignedOrderV2

// OrderV2 是未签名的 V2 订单。
type OrderV2 = obuilder.OrderV2

// ============================================================
//  常量
// ============================================================

// Bytes32Zero 32 字节零值的 hex 字符串表示。
const Bytes32Zero = "0x0000000000000000000000000000000000000000000000000000000000000000"

// 订单合约版本
const (
	OrderVersionV1 = 1
	OrderVersionV2 = 2
)

// SignatureTypeV2 V2 签名类型(对应 py-clob-client-v2.SignatureTypeV2)。
// 与 V1 的区别:V2 增加了 POLY_1271,用于 Deposit Wallet 智能合约钱包。
const (
	SigTypeEOA            = 0 // EOA / ECDSA EIP-712
	SigTypePolyProxy      = 1 // Polymarket Proxy 钱包(EOA 持有)
	SigTypePolyGnosisSafe = 2 // Polymarket Gnosis Safe(EOA 持有)
	SigTypePoly1271       = 3 // EIP-1271 智能合约钱包(V2 新增,用于 Deposit Wallet)
)

// V2 EIP-712 域常量
const (
	CTFExchangeV2DomainName    = "Polymarket CTF Exchange"
	CTFExchangeV2DomainVersion = "2"
	DepositWalletDomainName    = "DepositWallet"
	DepositWalletDomainVersion = "1"
)

// 订单版本不匹配错误标记(用于 _retry_on_version_update)
const OrderVersionMismatchError = "order_version_mismatch"

// ============================================================
//  V2 订单数据
// ============================================================

// OrderArgsV2 V2 限价订单输入(对应 py-clob-client-v2 OrderArgsV2)。
type OrderArgsV2 struct {
	TokenID    string  `json:"token_id"`
	Price      float64 `json:"price"`
	Size       float64 `json:"size"`
	Side       string  `json:"side"` // "BUY" or "SELL"
	Expiration int64   `json:"expiration"`              // 0 表示无过期
	BuilderCode string `json:"builder_code,omitempty"` // bytes32 hex, 默认 Bytes32Zero
	Metadata    string `json:"metadata,omitempty"`     // bytes32 hex, 默认 Bytes32Zero
	// 若提供,且为 BUY 单,SDK 会按 user_usdc_balance 扣除费用后调整 size。
	UserUsdcBalance *float64 `json:"user_usdc_balance,omitempty"`
}

// MarketOrderArgsV2 V2 市价订单输入。
type MarketOrderArgsV2 struct {
	TokenID         string    `json:"token_id"`
	Amount          float64   `json:"amount"` // BUY: USDC 金额; SELL: 份额
	Side            string    `json:"side"`
	Price           float64   `json:"price"` // 可选,默认根据 orderbook 计算
	OrderType       OrderType `json:"order_type"`
	UserUsdcBalance float64   `json:"user_usdc_balance,omitempty"`
	BuilderCode     string    `json:"builder_code,omitempty"`
	Metadata        string    `json:"metadata,omitempty"`
}

// OrderDataV2 是 SDK 顶层使用的 V2 订单输入(完整描述)。
// 真实的构造/签名逻辑发生在 order_builder.OrderDataV2,本类型保留
// 在公开 API 层是为了向调用者暴露类型(例如打印/序列化)。
type OrderDataV2 = obuilder.OrderDataV2

// PostOrdersArgsV2 V2 批量下单参数。
type PostOrdersArgsV2 struct {
	Order     *SignedOrderV2 `json:"order"`
	OrderType OrderType      `json:"orderType"`
	PostOnly  bool           `json:"postOnly,omitempty"`
	DeferExec bool           `json:"deferExec,omitempty"`
}

// ============================================================
//  服务端响应/辅助类型
// ============================================================

// BuilderConfig builder 配置(用于全局 builder code / 地址)。
type BuilderConfig struct {
	BuilderAddress string `json:"builder_address,omitempty"`
	BuilderCode    string `json:"builder_code,omitempty"`
}

// BuilderFeeRate builder 的 maker/taker 费率(单位:小数,如 0.001 = 0.1%)。
type BuilderFeeRate struct {
	Maker float64 `json:"maker"`
	Taker float64 `json:"taker"`
}

// BuilderFeeRateRaw 是 GET /fees/builder-fees/{code} 的原始响应。
// 服务端返回 bps 整数(10000 = 100%)。
type BuilderFeeRateRaw struct {
	BuilderMakerFeeRateBps int64 `json:"builder_maker_fee_rate_bps"`
	BuilderTakerFeeRateBps int64 `json:"builder_taker_fee_rate_bps"`
}

// BuilderFeesBps 是 builder fee rate 的基数。
const BuilderFeesBps = 10000

// FeeInfo 平台费率(rate, exponent)。
type FeeInfo struct {
	Rate     float64 `json:"rate"`
	Exponent float64 `json:"exponent"`
}

// FeeDetails 市场费率详情。
type FeeDetails struct {
	FeeRate   float64 `json:"r"`
	Exponent  int     `json:"e"`
	TakerOnly bool    `json:"taker_only"`
}

// ClobToken CLOB 市场内的一个 token (YES/NO)。
type ClobToken struct {
	TokenID string `json:"t"`
	Outcome string `json:"o,omitempty"`
}

// MarketDetails 缓存的市场详情,通过 /clob-markets/{conditionID} 返回。
// 字段名沿用服务端简写。
type MarketDetails struct {
	ConditionID                string      `json:"c"`
	Tokens                     []ClobToken `json:"t"`
	MinTickSize                string      `json:"mts"`
	NegRisk                    bool        `json:"nr"`
	FeeDetails                 *FeeDetails `json:"fd,omitempty"`
	MakerBaseFee               int         `json:"mbf,omitempty"`
	TakerBaseFee               int         `json:"tbf,omitempty"`
	AcceptingOrders            bool        `json:"accepting_orders,omitempty"`
	MinOrderSize               float64     `json:"min_order_size,omitempty"`
	SecondsDelay               int         `json:"seconds_delay,omitempty"`
	GameStartTime              string      `json:"game_start_time,omitempty"`
	ClearBookOnStart           bool        `json:"clear_book_on_start,omitempty"`
	AcceptingOrdersTimestamp   string      `json:"accepting_orders_timestamp,omitempty"`
	RFQEnabled                 bool        `json:"rfq_enabled,omitempty"`
	TakerOrderDelayEnabled     bool        `json:"taker_order_delay_enabled,omitempty"`
	BlockaidCheckEnabled       bool        `json:"blockaid_check_enabled,omitempty"`
}

// PriceHistoryInterval 价格历史时间区间。
type PriceHistoryInterval string

const (
	PriceHistory1m  PriceHistoryInterval = "1m"
	PriceHistory1h  PriceHistoryInterval = "1h"
	PriceHistory6h  PriceHistoryInterval = "6h"
	PriceHistory1d  PriceHistoryInterval = "1d"
	PriceHistory1w  PriceHistoryInterval = "1w"
	PriceHistoryMax PriceHistoryInterval = "max"
)

// PricesHistoryParams /prices-history 查询参数。
type PricesHistoryParams struct {
	Market   string               `json:"market"`
	StartTs  int64                `json:"startTs,omitempty"`
	EndTs    int64                `json:"endTs,omitempty"`
	Interval PriceHistoryInterval `json:"interval,omitempty"`
	Fidelity int                  `json:"fidelity,omitempty"`
}

// EarningsParams /rewards/user 查询参数。
type EarningsParams struct {
	Date string `json:"date"`
}

// RewardsMarketsParams /rewards/markets/{date} 查询参数。
type RewardsMarketsParams struct {
	Date    string `json:"date"`
	Cursor  string `json:"cursor,omitempty"`
	UserAddress string `json:"user_address,omitempty"`
}

// ============================================================
//  Builder API key 体系
// ============================================================
//
// 对齐 py-clob-client-v2/clob_types.py。Python V2 客户端方法本身不做反序列化,
// 直接返回 raw dict;这里给出 typed 视图,字段名按 Python 命名,同时提供
// ParseBuilderApiKey 容错地从原始响应里读出字段(兼容旧 api_* 命名)。

// BuilderApiKey 创建 builder API key 后服务端返回的完整凭证(只在 create
// 时出现一次,不可恢复)。
//
// 对应 Python clob_types.BuilderApiKey { key, secret, passphrase }。
type BuilderApiKey struct {
	Key        string `json:"key"`
	Secret     string `json:"secret"`
	Passphrase string `json:"passphrase"`
}

// BuilderApiKeyResponse 列表查询返回的单个 builder API key(不含 secret/passphrase)。
//
// 对应 Python clob_types.BuilderApiKeyResponse { key, created_at, revoked_at }。
type BuilderApiKeyResponse struct {
	Key       string `json:"key"`
	CreatedAt string `json:"created_at,omitempty"`
	RevokedAt string `json:"revoked_at,omitempty"`
}

// ParseBuilderApiKey 把 raw 响应解析为 BuilderApiKey,要求 **key、secret、
// passphrase 三个字段都非空**。这是 builder API key 的创建路径专用 —— secret
// 和 passphrase 在创建时只会返回一次,若解析时缺失,调用方将永久失去它们。
//
// 字段名兼容 V2 (key/secret/passphrase) 和旧版 (api_key/api_secret/api_passphrase)。
//
// 安全性:
//   - 错误信息只列出缺失字段名,绝不引用 raw map 的值。调用方
//     log.Fatal(err) / 上报 Sentry 时不会泄露 secret / passphrase。
//   - 同样 raw 也不在 fmt.Errorf 里 %v 出来。
func ParseBuilderApiKey(raw interface{}) (*BuilderApiKey, error) {
	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expected map response, got %T", raw)
	}
	pick := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := m[k]; ok {
				if s, ok := v.(string); ok && s != "" {
					return s
				}
			}
		}
		return ""
	}
	out := &BuilderApiKey{
		Key:        pick("key", "api_key", "apiKey"),
		Secret:     pick("secret", "api_secret", "secret_key"),
		Passphrase: pick("passphrase", "api_passphrase"),
	}
	var missing []string
	if out.Key == "" {
		missing = append(missing, "key")
	}
	if out.Secret == "" {
		missing = append(missing, "secret")
	}
	if out.Passphrase == "" {
		missing = append(missing, "passphrase")
	}
	if len(missing) > 0 {
		// 故意不打印 raw map —— 可能包含 secret/passphrase。
		return nil, fmt.Errorf("builder api key response is missing required field(s): %s", strings.Join(missing, ", "))
	}
	return out, nil
}

// ============================================================
//  其他
// ============================================================

// BanStatus /auth/ban-status/closed-only 响应。
type BanStatus struct {
	ClosedOnly bool `json:"closed_only"`
}

// OrderScoring /order-scoring 响应。
type OrderScoring struct {
	Scoring bool `json:"scoring"`
}

// PostOrderResultV2 V2 单笔下单返回。
type PostOrderResultV2 struct {
	Payload  map[string]interface{} `json:"payload"`
	Response interface{}            `json:"response"`
}

// PostOrdersResultV2 V2 批量下单返回。
type PostOrdersResultV2 struct {
	Payload  []map[string]interface{} `json:"payload"`
	Response interface{}              `json:"response"`
}

// SaltGenerator 用于生成订单 salt 的注入点(测试用)。
type SaltGenerator func() *big.Int
