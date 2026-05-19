package polymarket

// client_internal.go 收纳 V2 路径和 RFQ 路径都还在用的内部 helpers。
// V1 公共下单方法(CreateOrder / PostOrder 等)已在 V2 迁移中删除;但下面的
// 函数仍然是 SDK 内部基础设施。

import (
	"fmt"
	"math/big"
	"strconv"

	obuilder "github.com/0xNetuser/Polymarket-golang/polymarket/order_builder"
	"github.com/polymarket/go-order-utils/pkg/model"
)

// resolveTickSize 解析 tick size:用户传的优先,否则从市场拿。
// V2 路径(CreateOrderV2)通过 resolveTickAndNegRisk 间接调用。
func (c *ClobClient) resolveTickSize(tokenID string, tickSize *TickSize) (TickSize, error) {
	minTickSize, err := c.GetTickSize(tokenID)
	if err != nil {
		return "", err
	}
	if tickSize != nil {
		if IsTickSizeSmaller(*tickSize, minTickSize) {
			return "", fmt.Errorf("invalid tick size (%s), minimum for the market is %s", *tickSize, minTickSize)
		}
		return *tickSize, nil
	}
	return minTickSize, nil
}

// CalculateMarketPrice 按 orderbook 算市价。V2 CreateMarketOrderV2 / CreateAndPostMarketOrderV2
// 用得到,RFQ 也通过 createOrderV1ForRFQ 间接用。
func (c *ClobClient) CalculateMarketPrice(tokenID, side string, amount float64, orderType OrderType) (float64, error) {
	book, err := c.GetOrderBook(tokenID)
	if err != nil {
		return 0, fmt.Errorf("no orderbook: %w", err)
	}
	if side == BUY {
		if len(book.Asks) == 0 {
			return 0, fmt.Errorf("no match")
		}
		return c.builder.CalculateBuyMarketPrice(convertOrderSummaries(book.Asks), amount, string(orderType))
	}
	if len(book.Bids) == 0 {
		return 0, fmt.Errorf("no match")
	}
	return c.builder.CalculateSellMarketPrice(convertOrderSummaries(book.Bids), amount, string(orderType))
}

// convertOrderSummaries 已在 order_summary_wrapper.go 定义,这里不重复。

// createOrderV1ForRFQ 给 RFQ 用的 V1 订单构造。**只供 CreateOrderForRFQ 调用**,
// 不再 export 为公共 API —— 普通下单请用 CreateOrderV2。
//
// 为什么 RFQ 还在 V1:py-clob-client-v2 的 RFQ 子模块当前仍然构造 V1 订单
// (参见 ref/py-clob-client-v2/py_clob_client_v2/rfq/rfq_client.py:_build_v1_order)。
// 等 Polymarket V2 把 RFQ 也切到 V2,本函数就可以删。
func (c *ClobClient) createOrderV1ForRFQ(args *rfqOrderArgsV1) (*model.SignedOrder, error) {
	if err := c.assertLevel1Auth(); err != nil {
		return nil, err
	}
	if args == nil {
		return nil, fmt.Errorf("args is nil")
	}

	tickSize, err := c.resolveTickSize(args.TokenID, nil)
	if err != nil {
		return nil, err
	}
	negRisk, err := c.GetNegRisk(args.TokenID)
	if err != nil {
		return nil, err
	}
	feeRateBps, err := c.GetFeeRateBps(args.TokenID)
	if err != nil {
		return nil, err
	}

	if !PriceValid(args.Price, tickSize) {
		tsf, _ := strconv.ParseFloat(string(tickSize), 64)
		return nil, fmt.Errorf("invalid price (%v), min: %s - max: %v", args.Price, tickSize, 1-tsf)
	}

	roundConfig, ok := obuilder.RoundingConfig[string(tickSize)]
	if !ok {
		return nil, fmt.Errorf("unsupported tick size: %s", tickSize)
	}

	side, makerAmount, takerAmount, err := c.builder.GetOrderAmounts(args.Side, args.Size, args.Price, roundConfig)
	if err != nil {
		return nil, err
	}

	taker := args.Taker
	if taker == "" {
		taker = ZeroAddress
	}

	orderData := &model.OrderData{
		Maker:         c.builder.GetFunder(),
		Taker:         taker,
		TokenId:       args.TokenID,
		MakerAmount:   makerAmount.String(),
		TakerAmount:   takerAmount.String(),
		Side:          side,
		FeeRateBps:    strconv.Itoa(feeRateBps),
		Nonce:         "0",
		Signer:        c.signer.Address(),
		Expiration:    strconv.FormatInt(args.Expiration, 10),
		SignatureType: model.SignatureType(c.builder.GetSigType()),
	}

	cfg, err := GetContractConfig(c.chainID)
	if err != nil {
		return nil, err
	}
	exchangeAddr := cfg.GetExchangeForVersion(OrderVersionV1, negRisk)
	return c.builder.BuildSignedOrder(orderData, exchangeAddr, c.chainID, negRisk)
}

// rfqOrderArgsV1 是 createOrderV1ForRFQ 的入参,unexport 以表明这是 SDK 内部接口。
type rfqOrderArgsV1 struct {
	TokenID    string
	Price      float64
	Size       float64
	Side       string
	Expiration int64
	Taker      string
}

// 把 big.Int 引入用一下(go-order-utils.model.OrderData 已经引入了 big.Int 间接)
var _ = big.NewInt
