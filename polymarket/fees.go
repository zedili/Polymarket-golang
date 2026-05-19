package polymarket

// fees.go 对齐 py-clob-client-v2/py_clob_client_v2/fees.py。
//
// V2 在下单时支持两个新能力:
//   1. user_usdc_balance —— BUY 单可以传入用户余额,SDK 自动按服务端费率
//      公式 fee = (fee_rate * (price*(1-price))^exponent) 缩小订单 size,
//      让最终 maker_amount + 平台费 + builder taker 费 <= 余额。
//   2. fee_slippage —— 允许平台费率被放大百分比(用于上链时 fee 比预期高
//      的安全垫)。

import (
	"fmt"
	"math"
)

// MinFeeSlippagePercentage / MaxFeeSlippagePercentage 与 Python 一致。
const (
	MinFeeSlippagePercentage = 1.0
	MaxFeeSlippagePercentage = 100.0
)

// ValidateFeeSlippage 校验 fee_slippage 取值。
// 允许 0 或 [1, 100] 之间的百分比;其他值(包括 NaN、Inf、负数、小数 < 1)报错。
func ValidateFeeSlippage(feeSlippage float64) error {
	if math.IsNaN(feeSlippage) || math.IsInf(feeSlippage, 0) {
		return fmt.Errorf("fee_slippage must be 0 or a percentage between 1 and 100")
	}
	if feeSlippage < 0 || feeSlippage > MaxFeeSlippagePercentage {
		return fmt.Errorf("fee_slippage must be 0 or a percentage between 1 and 100")
	}
	if feeSlippage > 0 && feeSlippage < MinFeeSlippagePercentage {
		return fmt.Errorf("fee_slippage must be 0 or a percentage between 1 and 100")
	}
	return nil
}

// AdjustBuyAmountForFees 实现 py-clob-client-v2.fees.adjust_buy_amount_for_fees。
//
// 输入:
//   amount             —— 用户期望买入的 USDC 名义额(maker_amount in USDC)
//   price              —— 限价
//   userUsdcBalance    —— 用户当前 USDC 余额
//   feeRate / feeExp   —— 市场费率(0 = 无费)和指数
//   builderTakerFee    —— builder 代码对应的 taker 费率(0..1)
//   feeSlippage        —— 安全垫(百分比)
//
// 返回:经调整后的 amount。若余额够,原样返回 amount;若不够,返回能够覆盖
// 平台费 + builder 费后剩下的最大金额(不会小于 0)。
func AdjustBuyAmountForFees(
	amount, price, userUsdcBalance, feeRate, feeExp, builderTakerFee, feeSlippage float64,
) (float64, error) {
	if err := ValidateFeeSlippage(feeSlippage); err != nil {
		return 0, err
	}
	platformFeeRate := feeRate * math.Pow(price*(1-price), feeExp)
	effectivePlatformFeeRate := platformFeeRate * (1 + feeSlippage/100)
	feeBaseAmount := amount
	if userUsdcBalance < feeBaseAmount {
		feeBaseAmount = userUsdcBalance
	}
	platformFee := 0.0
	if price > 0 {
		platformFee = (feeBaseAmount / price) * effectivePlatformFeeRate
	}
	builderFee := feeBaseAmount * builderTakerFee
	totalCost := amount + platformFee + builderFee
	if userUsdcBalance <= totalCost {
		v := userUsdcBalance - platformFee - builderFee
		if v < 0 {
			v = 0
		}
		return v, nil
	}
	return amount, nil
}
