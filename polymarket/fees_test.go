package polymarket

import (
	"math"
	"testing"
)

// Golden 输出来自 py-clob-client-v2/fees.adjust_buy_amount_for_fees。
// 复现脚本:python3 /tmp/gen_market_and_fees_goldens.py
//
// 公式:platform_fee = (amount/price) * fee_rate * (price*(1-price))^exp * (1 + slippage/100)
//
//	builder_fee = min(amount, balance) * builder_taker_fee
//	total = amount + platform_fee + builder_fee
//	若 balance <= total: 返回 balance - platform_fee - builder_fee(>=0)
//	否则返回 amount。
func TestAdjustBuyAmountForFees_MatchesPython(t *testing.T) {
	cases := []struct {
		name                                                                     string
		amount, price, balance, feeRate, feeExp, builderTakerFee, slippage, want float64
	}{
		{"balance_enough", 100, 0.5, 500, 0.02, 1, 0.01, 0, 100},
		{"balance_just_enough", 100, 0.5, 101.5, 0.02, 1, 0.01, 0, 99.5},
		{"balance_low", 100, 0.5, 50, 0.02, 1, 0.01, 0, 49.0},
		{"zero_rate", 100, 0.5, 80, 0, 1, 0, 0, 80.0},
		{"with_slippage", 100, 0.3, 80, 0.02, 1, 0.01, 10, 77.968},
		{"no_builder", 100, 0.7, 50, 0.015, 2, 0, 0, 49.95275},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := AdjustBuyAmountForFees(c.amount, c.price, c.balance, c.feeRate, c.feeExp, c.builderTakerFee, c.slippage)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if math.Abs(got-c.want) > 1e-9 {
				t.Errorf("got %.10f, want %.10f", got, c.want)
			}
		})
	}
}

func TestValidateFeeSlippage(t *testing.T) {
	valid := []float64{0, 1, 50, 100}
	for _, v := range valid {
		if err := ValidateFeeSlippage(v); err != nil {
			t.Errorf("valid value %v rejected: %v", v, err)
		}
	}
	invalid := []float64{-1, 0.5, 100.1, math.NaN(), math.Inf(1)}
	for _, v := range invalid {
		if err := ValidateFeeSlippage(v); err == nil {
			t.Errorf("invalid value %v should have been rejected", v)
		}
	}
}

func TestClobClientSetFeeSlippage(t *testing.T) {
	creds := &ApiCreds{APIKey: "k", APISecret: testSecret, APIPassphrase: "p"}
	c, err := NewClobClient("http://127.0.0.1:1", 137, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "", creds, nil, "")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if c.GetFeeSlippage() != 0 {
		t.Errorf("default fee slippage should be 0")
	}
	if err := c.SetFeeSlippage(5); err != nil {
		t.Errorf("set 5: %v", err)
	}
	if c.GetFeeSlippage() != 5 {
		t.Errorf("set/get mismatch")
	}
	if err := c.SetFeeSlippage(-1); err == nil {
		t.Errorf("expected -1 to be rejected")
	}
}
