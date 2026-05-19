package web3

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// 已知的两类 collateral —— V2 升级之后链上共存:
//   - USDC.e (V1)  0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174
//   - pUSD   (V2)  0xC011a7E12a19f7B1f670d46F03B03f3342E82DFB
//
// 一个 condition 创建时绑定的 collateral 不可变,所以 redeem 时 collateral
// 参数必须与创建时一致。Polymarket V1 时代(2026-05 前)所有 condition 都是
// USDC.e;V2 之后新 condition 改为 pUSD。
var (
	CollateralUSDCe = common.HexToAddress("0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174")
	CollateralPUSD  = common.HexToAddress("0xC011a7E12a19f7B1f670d46F03B03f3342E82DFB")
)

// ComputePositionID 本地算 ConditionalTokens 的 positionId(ERC1155 token id)。
//
//   positionId = uint256(keccak256(abi.encodePacked(collateralToken, collectionId)))
//
// 注意:**collectionId 不能本地算** —— Gnosis CTF 的 getCollectionId 涉及 BN256
// 椭圆曲线点加法处理 parentCollectionId,必须通过 chain RPC 拿到。
// 但 getPositionId 是简单 keccak,可以本地计算。
func ComputePositionID(collateral common.Address, collectionID common.Hash) *big.Int {
	var buf [52]byte
	copy(buf[0:20], collateral[:])
	copy(buf[20:52], collectionID[:])
	return new(big.Int).SetBytes(crypto.Keccak256(buf[:]))
}

// ChainGetCollectionID 通过 RPC 调链上 ConditionalTokens.getCollectionId。
// 这是必须的:本地无法复现这个算法(BN256 ECC 加法)。
func (c *BaseWeb3Client) ChainGetCollectionID(
	parentCollectionID common.Hash,
	conditionID common.Hash,
	indexSet *big.Int,
) (common.Hash, error) {
	data, err := ConditionalTokensABI.Pack("getCollectionId", parentCollectionID, conditionID, indexSet)
	if err != nil {
		return common.Hash{}, fmt.Errorf("pack getCollectionId: %w", err)
	}
	result, err := c.callContract(context.Background(), &c.ConditionalTokensAddress, data)
	if err != nil {
		return common.Hash{}, fmt.Errorf("call getCollectionId: %w", err)
	}
	var out [32]byte
	if err := ConditionalTokensABI.UnpackIntoInterface(&out, "getCollectionId", result); err != nil {
		return common.Hash{}, fmt.Errorf("unpack getCollectionId: %w", err)
	}
	return common.Hash(out), nil
}

// ResolveCollateralFromAsset 在 V1 USDC.e 与 V2 pUSD 之间反推:这个 condition
// 当初是用哪个 collateral 创建的?
//
// 算法(每条 condition 仅 2 次 RPC):
//  1. 从链上拿 outcome 0 (indexSet=1) 和 outcome 1 (indexSet=2) 的 collectionId
//  2. 本地算 USDC.e 与 pUSD 下的 4 个 positionId
//  3. 与调用方提供的 knownAsset (data-api 给的 token id)比对,匹配的 collateral 就是答案
//
// 失败 fallback 时返回 fallback 参数(推荐 CollateralUSDCe,因为 V2 升级前的
// 历史 condition 占绝大多数)。
func (c *BaseWeb3Client) ResolveCollateralFromAsset(
	conditionID common.Hash,
	knownAsset *big.Int,
	fallback common.Address,
) (common.Address, error) {
	if knownAsset == nil {
		return fallback, fmt.Errorf("knownAsset is nil")
	}

	collIDs := make([]common.Hash, 0, 2)
	for _, is := range []int64{1, 2} {
		id, err := c.ChainGetCollectionID(common.Hash{}, conditionID, big.NewInt(is))
		if err != nil {
			// 单次 RPC 失败不致命 —— 仍可能用另一个 outcome 匹配
			continue
		}
		collIDs = append(collIDs, id)
	}
	if len(collIDs) == 0 {
		return fallback, fmt.Errorf("failed to fetch any collectionId for condition %s", conditionID.Hex())
	}

	for _, cand := range []common.Address{CollateralUSDCe, CollateralPUSD} {
		for _, cid := range collIDs {
			if ComputePositionID(cand, cid).Cmp(knownAsset) == 0 {
				return cand, nil
			}
		}
	}
	return fallback, fmt.Errorf("knownAsset %s did not match any (collateral, outcome) for condition %s",
		knownAsset.String(), conditionID.Hex())
}

// ResolveCollateralFromAssetBatch 是 ResolveCollateralFromAsset 的并发批量版本。
// 对每个 input 启一个 goroutine(受 semaphore 限制最多 maxConcurrency 同时跑),
// 按 index 返回结果(失败时该位置为 fallback)。
//
// 不限流地一次发几百个 RPC 几乎一定会被 public RPC provider 限流到 429,反而
// 比串行更慢。默认 maxConcurrency = 8,适合 publicnode / quicknode 公共节点;
// 自托管节点可以调高(传 0 等同默认)。
type CollateralQuery struct {
	ConditionID common.Hash
	KnownAsset  *big.Int
}

// DefaultCollateralDetectConcurrency 默认并发上限。
const DefaultCollateralDetectConcurrency = 8

func (c *BaseWeb3Client) ResolveCollateralFromAssetBatch(
	queries []CollateralQuery,
	fallback common.Address,
) []common.Address {
	return c.ResolveCollateralFromAssetBatchWithLimit(queries, fallback, DefaultCollateralDetectConcurrency)
}

// ResolveCollateralFromAssetBatchWithLimit 同上,自定义并发上限。
// maxConcurrency <= 0 时退化为默认值。
func (c *BaseWeb3Client) ResolveCollateralFromAssetBatchWithLimit(
	queries []CollateralQuery,
	fallback common.Address,
	maxConcurrency int,
) []common.Address {
	if maxConcurrency <= 0 {
		maxConcurrency = DefaultCollateralDetectConcurrency
	}
	out := make([]common.Address, len(queries))
	sem := make(chan struct{}, maxConcurrency)
	done := make(chan struct{}, len(queries))
	// 关键:sem <- 必须在 `go` 之前。否则 100k queries 会瞬间 spawn 100k goroutine,
	// 全堵在 `sem <- struct{}{}` 内部,等于失去并发限制 + 一次性占用 ~80MiB 栈。
	for i, q := range queries {
		sem <- struct{}{}
		go func(i int, q CollateralQuery) {
			defer func() { <-sem }()
			coll, _ := c.ResolveCollateralFromAsset(q.ConditionID, q.KnownAsset, fallback)
			out[i] = coll
			done <- struct{}{}
		}(i, q)
	}
	for range queries {
		<-done
	}
	return out
}

// 静态检查:ConditionalTokensABI 必须有 getCollectionId 方法。
// 如果 ABI JSON 里缺这个方法,SDK 启动就会报错,而不是 redeem 时才发现。
func init() {
	if _, ok := ConditionalTokensABI.Methods["getCollectionId"]; !ok {
		// 不 panic —— 让用户能用其它功能,只是 ResolveCollateralFromAsset 会失败。
		// 用 SDK Logger 而不是 fmt.Println 避免污染调用方 stdout。
		logf("warning: ConditionalTokensABI missing getCollectionId; collateral auto-detect unavailable")
	}
}

// 保留这个未使用的 import 以防未来 abi 包用到
var _ = abi.NewType
