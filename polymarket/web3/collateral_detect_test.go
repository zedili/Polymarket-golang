package web3

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// TestComputePositionID_MatchesChain 验证本地 keccak(packed(addr, collId))
// 与链上 ConditionalTokens.getPositionId 完全一致。
//
// Golden 数据来自真实 Polymarket condition + 链上 RPC 调用(2026-05-18):
//   condition: 0x016bae70868feddc1ef9021fe6dd1be98b2bcf49e00856e4691c6503867e8a43
//   chain getCollectionId(0, cond, indexSet=1) = 0x52c295b1...
//   chain getPositionId(USDC.e, collId)        = 3739476109831327132881821621112738323041936388232160169720641156959773678299
func TestComputePositionID_MatchesChain(t *testing.T) {
	// 注意:collectionId 必须从链上拿(无法本地复现),这里直接用 chain 给的值
	collID := common.HexToHash("0x52c295b11ee00010a83e7dc1b92f0539a6e7a0fb939fcb8f0b9849f75b175df7")
	want, _ := new(big.Int).SetString("3739476109831327132881821621112738323041936388232160169720641156959773678299", 10)
	got := ComputePositionID(CollateralUSDCe, collID)
	if got.Cmp(want) != 0 {
		t.Errorf("positionId mismatch:\n  got  %s\n  want %s", got.String(), want.String())
	}
}

// TestComputePositionID_DifferentCollateralsDiffer 同一 collectionId 在不同
// collateral 下必须算出不同 positionId(否则 V1 token 会被当 V2 token 处理)。
func TestComputePositionID_DifferentCollateralsDiffer(t *testing.T) {
	collID := common.HexToHash("0x52c295b11ee00010a83e7dc1b92f0539a6e7a0fb939fcb8f0b9849f75b175df7")
	v1 := ComputePositionID(CollateralUSDCe, collID)
	v2 := ComputePositionID(CollateralPUSD, collID)
	if v1.Cmp(v2) == 0 {
		t.Error("expected USDC.e and pUSD positionIds to differ")
	}
}

func TestCollateralConstants(t *testing.T) {
	if CollateralUSDCe.Hex() != "0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174" {
		t.Errorf("USDC.e address changed: %s", CollateralUSDCe.Hex())
	}
	if CollateralPUSD.Hex() != "0xC011a7E12a19f7B1f670d46F03B03f3342E82DFB" {
		t.Errorf("pUSD address changed: %s", CollateralPUSD.Hex())
	}
}

func TestConditionalTokensABIHasGetCollectionID(t *testing.T) {
	// ResolveCollateralFromAsset 依赖这个方法。如果 ABI JSON 没暴露,SDK 启动
	// 会 print warning(见 init()),但测试这里直接断言。
	if _, ok := ConditionalTokensABI.Methods["getCollectionId"]; !ok {
		t.Fatal("ConditionalTokensABI is missing getCollectionId method")
	}
}
