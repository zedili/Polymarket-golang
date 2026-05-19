package polymarket

// API端点常量
// 与 py-clob-client-v2 对齐(2026-05),并保留若干 V1 端点以维持向后兼容。
const (
	// 健康/版本
	OK      = "/ok"      // V2 新增
	Time    = "/time"
	Version = "/version" // V2 新增

	// API密钥管理 (L1)
	CreateAPIKey = "/auth/api-key"
	GetAPIKeys   = "/auth/api-keys"
	DeleteAPIKey = "/auth/api-key"
	DeriveAPIKey = "/auth/derive-api-key"
	ClosedOnly   = "/auth/ban-status/closed-only"

	// 只读API密钥
	CreateReadonlyAPIKey   = "/auth/readonly-api-key"
	GetReadonlyAPIKeys     = "/auth/readonly-api-keys"
	DeleteReadonlyAPIKey   = "/auth/readonly-api-key"
	ValidateReadonlyAPIKey = "/auth/validate-readonly-api-key" // V1 兼容,V2 已移除

	// Builder API key (V2 新增)
	CreateBuilderAPIKey       = "/auth/builder-api-key"
	GetBuilderAPIKeysEndpoint = "/auth/builder-api-key"
	RevokeBuilderAPIKey       = "/auth/builder-api-key"

	// 交易和订单
	Trades             = "/data/trades"
	PreMigrationOrders = "/data/pre-migration-orders" // V2 新增
	GetOrderBook       = "/book"
	GetOrderBooks      = "/books"
	GetOrder           = "/data/order/"
	Orders             = "/data/orders"
	PostOrder          = "/order"
	PostOrders         = "/orders"
	Cancel             = "/order"
	CancelOrders       = "/orders"
	CancelAll          = "/cancel-all"
	CancelMarketOrders = "/cancel-market-orders"

	// 价格和市场数据
	MidPoint            = "/midpoint"
	MidPoints           = "/midpoints"
	Price               = "/price"
	GetPrices           = "/prices"
	GetSpread           = "/spread"
	GetSpreads          = "/spreads"
	GetLastTradePrice   = "/last-trade-price"
	GetLastTradesPrices = "/last-trades-prices"
	GetPricesHistory    = "/prices-history" // V2 新增

	// 通知
	GetNotifications  = "/notifications"
	DropNotifications = "/notifications"

	// 余额和授权
	GetBalanceAllowance    = "/balance-allowance"
	UpdateBalanceAllowance = "/balance-allowance/update"

	// 订单评分
	IsOrderScoring   = "/order-scoring"
	AreOrdersScoring = "/orders-scoring"

	// 市场信息
	GetTickSize                  = "/tick-size"
	GetNegRisk                   = "/neg-risk"
	GetFeeRate                   = "/fee-rate"
	GetSamplingSimplifiedMarkets = "/sampling-simplified-markets"
	GetSamplingMarkets           = "/sampling-markets"
	GetSimplifiedMarkets         = "/simplified-markets"
	GetMarkets                   = "/markets"
	GetMarket                    = "/markets/"
	GetMarketByToken             = "/markets-by-token/"      // V2 新增
	GetClobMarket                = "/clob-markets/"          // V2 新增
	GetMarketTradesEvents        = "/markets/live-activity/" // V2 调整路径

	// Builder
	GetBuilderTrades  = "/builder/trades"
	GetBuilderFeeRate = "/fees/builder-fees/" // V2 新增

	// Heartbeat
	PostHeartbeat = "/v1/heartbeats"

	// Rewards (V2 新增公开方法)
	GetEarningsForUserForDay      = "/rewards/user"
	GetTotalEarningsForUserForDay = "/rewards/user/total"
	GetLiquidityRewardPercentages = "/rewards/user/percentages"
	GetRewardsMarketsCurrent      = "/rewards/markets/current"
	GetRewardsMarketsEndpoint     = "/rewards/markets/"
	GetRewardsEarningsPercentages = "/rewards/user/markets"

	// Rebates
	GetCurrentMakerRebate = "/rebates/current"

	// RFQ
	CreateRFQRequest      = "/rfq/request"
	CancelRFQRequest      = "/rfq/request"
	GetRFQRequests        = "/rfq/data/requests"
	CreateRFQQuote        = "/rfq/quote"
	CancelRFQQuote        = "/rfq/quote"
	GetRFQRequesterQuotes = "/rfq/data/requester/quotes"
	GetRFQQuoterQuotes    = "/rfq/data/quoter/quotes"
	GetRFQBestQuote       = "/rfq/data/best-quote"
	RFQRequestsAccept     = "/rfq/request/accept"
	RFQQuoteApprove       = "/rfq/quote/approve"
	RFQConfig             = "/rfq/config"
)
