package polymarket

import (
	"strings"
	"testing"
)

// TestContractConfigCollateralIsPUSD —— V2 collateral 必须是 pUSD,
// 不能是 USDC.e (0x2791...)。Polygon 链上 pUSD 的 symbol = "pUSD",6 decimals。
// 这条断言用来防止有人不小心把 collateral 改回 USDC.e。
func TestContractConfigCollateralIsPUSD(t *testing.T) {
	const wantPUSD = "0xC011a7E12a19f7B1f670d46F03B03f3342E82DFB"
	for _, chainID := range []int{137, 80002} {
		cfg, err := GetContractConfig(chainID)
		if err != nil {
			t.Fatalf("chain %d: %v", chainID, err)
		}
		if !strings.EqualFold(cfg.Collateral, wantPUSD) {
			t.Errorf("chain %d Collateral = %s, want pUSD %s", chainID, cfg.Collateral, wantPUSD)
		}
	}
}

func TestContractConfigPolygonV2(t *testing.T) {
	cfg, err := GetContractConfig(137)
	if err != nil {
		t.Fatalf("polygon: %v", err)
	}
	wantV2 := "0xE111180000d2663C0091e4f400237545B87B996B"
	wantNegRiskV2 := "0xe2222d279d744050d28e00520010520000310F59"
	if !strings.EqualFold(cfg.ExchangeV2, wantV2) {
		t.Errorf("ExchangeV2 = %s, want %s", cfg.ExchangeV2, wantV2)
	}
	if !strings.EqualFold(cfg.NegRiskExchangeV2, wantNegRiskV2) {
		t.Errorf("NegRiskExchangeV2 = %s, want %s", cfg.NegRiskExchangeV2, wantNegRiskV2)
	}
	if cfg.GetExchangeForVersion(OrderVersionV2, false) != cfg.ExchangeV2 {
		t.Error("V2 + negRisk=false should return ExchangeV2")
	}
	if cfg.GetExchangeForVersion(OrderVersionV2, true) != cfg.NegRiskExchangeV2 {
		t.Error("V2 + negRisk=true should return NegRiskExchangeV2")
	}
	if cfg.GetExchangeForVersion(OrderVersionV1, false) != cfg.Exchange {
		t.Error("V1 + negRisk=false should return Exchange")
	}
	if cfg.GetExchangeForVersion(OrderVersionV1, true) != cfg.NegRiskExchange {
		t.Error("V1 + negRisk=true should return NegRiskExchange")
	}
}

func TestContractConfigAmoy(t *testing.T) {
	cfg, err := GetContractConfig(80002)
	if err != nil {
		t.Fatalf("amoy: %v", err)
	}
	if cfg.ExchangeV2 == "" {
		t.Error("Amoy must have ExchangeV2 populated")
	}
	if cfg.Exchange == "" {
		t.Error("Amoy must keep V1 Exchange populated")
	}
}

func TestContractConfigUnknownChain(t *testing.T) {
	if _, err := GetContractConfig(1); err == nil {
		t.Error("expected error for unknown chain id")
	}
}

func TestContractConfigReturnsCopy(t *testing.T) {
	cfg1, _ := GetContractConfig(137)
	cfg1.ExchangeV2 = "tampered"
	cfg2, _ := GetContractConfig(137)
	if cfg2.ExchangeV2 == "tampered" {
		t.Error("GetContractConfig should return a defensive copy")
	}
}
