package web3

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// CollateralOnramp —— Polygon mainnet, permissionless USDC.e → pUSD 包装合约。
//
// 文档: https://docs.polymarket.com/concepts/pusd
// 接口:
//
//	function wrap(address _asset, address _to, uint256 _amount)
//
// 调用方需先 approve CollateralOnramp(注意:approve 给 onramp,不是 pUSD)。
// 调用后 onramp 把 _amount 单位的 USDC.e(6 dec)从 caller 转走,等量 mint pUSD 给 _to。
//
// 典型场景:V1 USDC.e 条件 redeem 后,proxy 钱包里多出来的 USDC.e 走这个把它
// wrap 成 pUSD,Polymarket UI 上会消掉 "Pending deposit" 提示。
var CollateralOnramp = common.HexToAddress("0x93070a847efEf7F70739046A929D47a521F5B8ee")

// CollateralOnrampABI 最小 ABI —— 只需要 wrap()。
var CollateralOnrampABI abi.ABI

const collateralOnrampABIJSON = `[
  {
    "type": "function",
    "name": "wrap",
    "stateMutability": "nonpayable",
    "inputs": [
      { "name": "_asset",  "type": "address" },
      { "name": "_to",     "type": "address" },
      { "name": "_amount", "type": "uint256" }
    ],
    "outputs": []
  }
]`

func init() {
	var err error
	CollateralOnrampABI, err = abi.JSON(strings.NewReader(collateralOnrampABIJSON))
	if err != nil {
		panic("failed to parse CollateralOnramp ABI: " + err.Error())
	}
}

// EncodeWrapUSDCe 构造 onramp.wrap(USDCe, to, amount) 的 calldata。
//   - to     : 接收 pUSD 的地址(对 proxy 用户填 proxy 地址自己)
//   - amount : USDC.e 的最小单位(6 dec),比如 1 USDC.e = 1_000_000
func EncodeWrapUSDCe(to common.Address, amount *big.Int) ([]byte, error) {
	if amount == nil || amount.Sign() <= 0 {
		return nil, fmt.Errorf("wrap amount must be > 0")
	}
	return CollateralOnrampABI.Pack("wrap", CollateralUSDCe, to, amount)
}

// EncodeApproveUSDCeToOnramp 构造 USDC.e.approve(CollateralOnramp, amount) 的 calldata。
// 注意:必须 approve 给 CollateralOnramp,不是 pUSD 合约本身(这是文档明确指出的常见错误)。
func EncodeApproveUSDCeToOnramp(amount *big.Int) ([]byte, error) {
	if amount == nil || amount.Sign() < 0 {
		return nil, fmt.Errorf("approve amount must be >= 0")
	}
	return USDCABI.Pack("approve", CollateralOnramp, amount)
}
