package polymarket

import "fmt"

// getContractConfig 获取链的合约配置(已升级为同时返回 V1 + V2 地址)。
//
// 历史上该函数有第二个参数 `negRisk`,根据 negRisk 切换 Exchange 字段。这种
// 设计在 V2 上线之后已经无法表达全部合约,所以 V2 之后改为「一个 chainID 对应
// 一份完整的 ContractConfig」,调用方再用 ContractConfig.GetExchangeForVersion
// 选合约。
//
// negRisk 参数被保留是为了 SDK 内部 V1 旧代码暂时可用 —— 但实际上无论 negRisk
// 取何值都返回同一份完整配置,negRisk 仅影响调用方如何取字段。
func getContractConfig(chainID int, negRisk bool) *ContractConfig {
	_ = negRisk
	cfg, err := GetContractConfig(chainID)
	if err != nil {
		panic(err.Error())
	}
	return cfg
}

// GetContractConfig 是导出的合约配置访问器(V2 推荐入口)。
// 地址来源:py-clob-client-v2/py_clob_client_v2/config.py
func GetContractConfig(chainID int) (*ContractConfig, error) {
	cfg, ok := contractConfigs[chainID]
	if !ok {
		return nil, fmt.Errorf("invalid chain_id: %d", chainID)
	}
	clone := *cfg
	return &clone, nil
}

var contractConfigs = map[int]*ContractConfig{
	// Polygon mainnet
	//
	// Collateral 现在是 pUSD (Polymarket USD, 6 decimals):
	//   0xC011a7E12a19f7B1f670d46F03B03f3342E82DFB
	// V2 把抵押品从 USDC.e (0x2791...) 换成了 pUSD。链上确认:
	//   name()    = "Polymarket USD"
	//   symbol()  = "pUSD"
	//   decimals  = 6
	// 旧 USDC.e (0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174) 已不再被 V2 服务端
	// 和 Exchange 合约使用。
	137: {
		Exchange:          "0x4bFb41d5B3570DeFd03C39a9A4D8dE6Bd8B8982E",
		NegRiskExchange:   "0xC5d563A36AE78145C45a50134d48A1215220f80a",
		ExchangeV2:        "0xE111180000d2663C0091e4f400237545B87B996B",
		NegRiskExchangeV2: "0xe2222d279d744050d28e00520010520000310F59",
		Collateral:        "0xC011a7E12a19f7B1f670d46F03B03f3342E82DFB", // pUSD
		ConditionalTokens: "0x4D97DCd97eC945f40cF65F87097ACe5EA0476045",
		NegRiskAdapter:    "0xd91E80cF2E7be2e162c6513ceD06f1dD0dA35296",
	},
	// Amoy testnet —— py-clob-client-v2 也使用同一个 pUSD 地址。
	80002: {
		Exchange:          "0xdFE02Eb6733538f8Ea35D585af8DE5958AD99E40",
		NegRiskExchange:   "0xC5d563A36AE78145C45a50134d48A1215220f80a",
		ExchangeV2:        "0xE111180000d2663C0091e4f400237545B87B996B",
		NegRiskExchangeV2: "0xe2222d279d744050d28e00520010520000310F59",
		Collateral:        "0xC011a7E12a19f7B1f670d46F03B03f3342E82DFB", // pUSD
		ConditionalTokens: "0x69308FB512518e39F9b16112fA8d994F4e2Bf8bB",
		NegRiskAdapter:    "0xd91E80cF2E7be2e162c6513ceD06f1dD0dA35296",
	},
}
