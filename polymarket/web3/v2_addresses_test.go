package web3

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// V1 NegRiskExchange 必须是 0xC5d563...,不能是 NegRiskAdapter (0xd91E...)。
// 历史上 Amoy 配置写错过,这条断言防止回归。
func TestChainConfigV1NegRiskExchangeMatchesPython(t *testing.T) {
	const wantV1NegRisk = "0xC5d563A36AE78145C45a50134d48A1215220f80a"
	const adapter = "0xd91E80cF2E7be2e162c6513ceD06f1dD0dA35296"
	for _, chainID := range []int64{137, 80002} {
		cfg := chainConfigs[chainID]
		got := cfg.NegRiskExchange.Hex()
		if !strings.EqualFold(got, wantV1NegRisk) {
			t.Errorf("chain %d NegRiskExchange = %s, want %s", chainID, got, wantV1NegRisk)
		}
		if strings.EqualFold(got, adapter) {
			t.Errorf("chain %d NegRiskExchange was confused with NegRiskAdapter", chainID)
		}
	}
}

// 验证 V2 Exchange 地址已被正确写入链配置。
func TestChainConfigCarriesV2Addresses(t *testing.T) {
	wantExchangeV2 := "0xE111180000d2663C0091e4f400237545B87B996B"
	wantNegRiskV2 := "0xe2222d279d744050d28e00520010520000310F59"

	for _, chainID := range []int64{137, 80002} {
		cfg, ok := chainConfigs[chainID]
		if !ok {
			t.Fatalf("missing chain config for %d", chainID)
		}
		if !strings.EqualFold(cfg.ExchangeV2.Hex(), wantExchangeV2) {
			t.Errorf("chain %d ExchangeV2 = %s, want %s", chainID, cfg.ExchangeV2.Hex(), wantExchangeV2)
		}
		if !strings.EqualFold(cfg.NegRiskExchangeV2.Hex(), wantNegRiskV2) {
			t.Errorf("chain %d NegRiskExchangeV2 = %s, want %s", chainID, cfg.NegRiskExchangeV2.Hex(), wantNegRiskV2)
		}
	}
}

// 验证 BaseWeb3Client 把 V2 地址挂到了实例字段上。
func TestBaseWeb3ClientHasV2Addresses(t *testing.T) {
	pk := "0x" + strings.Repeat("aa", 32)
	c, err := NewBaseWeb3Client(pk, SignatureTypeEOA, 137, "https://polygon-rpc.com")
	if err != nil {
		t.Skipf("RPC dial failed (offline?): %v", err)
	}
	if (c.ExchangeV2Address == common.Address{}) {
		t.Error("ExchangeV2Address should be populated")
	}
	if (c.NegRiskExchangeV2Address == common.Address{}) {
		t.Error("NegRiskExchangeV2Address should be populated")
	}
	if c.ExchangeV2Address == c.ExchangeAddress {
		t.Error("V2 exchange should differ from V1")
	}
}
