package order_builder

import (
	"testing"
)

// Goldens from py-clob-client-v2 (V2 builder).
// Reproduced by: python3 /tmp/gen_market_and_fees_goldens.py
//
// 关键的 market_buy_round_down_price 用 price=0.555,tick=0.01:
//   - V2 round_down(0.555, 2) = 0.55 → taker = 100/0.55 ≈ 181.818181 → 181818100
//   - V1 round_normal(0.555, 2) = 0.56 → taker = 100/0.56 ≈ 178.571 (不同!)
// 因此本测试能保证 V2 path 没有 fallback 到 V1 行为。
func TestGetOrderAmountsV2_Limit(t *testing.T) {
	cases := []struct {
		name                string
		side                string
		size, price         float64
		tick                string
		wantSide            uint8
		wantMaker, wantTaker string
	}{
		{"limit_buy_basic", "BUY", 100, 0.5, "0.01", 0, "50000000", "100000000"},
		{"limit_sell_basic", "SELL", 100, 0.5, "0.01", 1, "100000000", "50000000"},
		{"limit_buy_tricky_price", "BUY", 7.31, 0.13, "0.01", 0, "950300", "7310000"},
		{"limit_sell_tricky", "SELL", 3.27, 0.789, "0.001", 1, "3270000", "2580030"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := RoundingConfig[c.tick]
			s, m, tk, err := GetOrderAmountsV2(c.side, c.size, c.price, cfg)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if s != c.wantSide || m.String() != c.wantMaker || tk.String() != c.wantTaker {
				t.Errorf("got (side=%d maker=%s taker=%s) want (side=%d maker=%s taker=%s)",
					s, m.String(), tk.String(), c.wantSide, c.wantMaker, c.wantTaker)
			}
		})
	}
}

func TestGetMarketOrderAmountsV2_RoundDownPrice(t *testing.T) {
	cases := []struct {
		name                string
		side                string
		amount, price       float64
		tick                string
		wantSide            uint8
		wantMaker, wantTaker string
	}{
		// price=0.555 round_down=0.55: 100/0.55 = 181.81818... → 181818100
		{"market_buy_round_down_price", "BUY", 100, 0.555, "0.01", 0, "100000000", "181818100"},
		// price=0.789 round_down=0.78: 50*0.78 = 39 → 39000000
		{"market_sell_round_down_price", "SELL", 50, 0.789, "0.01", 1, "50000000", "39000000"},
		{"market_buy_finer_tick", "BUY", 100, 0.5555, "0.001", 0, "100000000", "180180180"},
		{"market_buy_already_tick", "BUY", 100, 0.5, "0.01", 0, "100000000", "200000000"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := RoundingConfig[c.tick]
			s, m, tk, err := GetMarketOrderAmountsV2(c.side, c.amount, c.price, cfg)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if s != c.wantSide || m.String() != c.wantMaker || tk.String() != c.wantTaker {
				t.Errorf("got (side=%d maker=%s taker=%s) want (side=%d maker=%s taker=%s)",
					s, m.String(), tk.String(), c.wantSide, c.wantMaker, c.wantTaker)
			}
		})
	}
}

func TestAmountsV2_RejectsBadSide(t *testing.T) {
	cfg := RoundingConfig["0.01"]
	if _, _, _, err := GetOrderAmountsV2("HOLD", 1, 0.5, cfg); err == nil {
		t.Error("expected error for invalid side")
	}
	if _, _, _, err := GetMarketOrderAmountsV2("HOLD", 1, 0.5, cfg); err == nil {
		t.Error("expected error for invalid side")
	}
}
