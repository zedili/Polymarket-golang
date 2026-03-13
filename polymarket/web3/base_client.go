package web3

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// 常量地址
var (
	AddressZero = common.HexToAddress("0x0000000000000000000000000000000000000000")
	HashZero    = common.Hash{}

	// Polygon 主网合约地址
	NegRiskAdapterAddress   = common.HexToAddress("0xd91E80cF2E7be2e162c6513ceD06f1dD0dA35296")
	ProxyFactoryAddress     = common.HexToAddress("0xaB45c5A4B0c941a2F231C04C3f49182e1A254052")
	SafeProxyFactoryAddress = common.HexToAddress("0xaacFeEa03eb1561C4e67d661e40682Bd20E3541b")

	// 默认 RPC 端点
	DefaultPolygonRPC = "https://polygon-rpc.com"
)

// callGasFeeCap 和 callGasTipCap 用于 eth_call/estimateGas。
// Polygon Bor v2.6.0 升级后，节点的 CallDefaults 在 baseFee 存在时，
// 会为未设置的 maxFeePerGas/maxPriorityFeePerGas 填入极低的默认值，导致 baseFee 校验失败。
//
// 解决方案：同时显式设置 GasFeeCap 和 GasTipCap 为 0。
// 当两者都为 0 时，EVM 的 state_transition 会触发 skipCheck：
//   skipCheck := st.evm.Config.NoBaseFee && msg.GasFeeCap.BitLen() == 0 && msg.GasTipCap.BitLen() == 0
// eth_call 的 NoBaseFee=true，所以 skipCheck=true，完全跳过 baseFee 校验。
// 关键：必须同时设置两个字段（非 nil），否则节点会填入自己的默认值。
var (
	callGasFeeCap = new(big.Int) // 0 - 显式设置，阻止节点填默认值
	callGasTipCap = new(big.Int) // 0 - 显式设置，阻止节点填默认值
)

// ChainConfig 链配置
type ChainConfig struct {
	ChainID           int64
	Exchange          common.Address
	Collateral        common.Address
	ConditionalTokens common.Address
	NegRiskExchange   common.Address
}

// 链配置映射
var chainConfigs = map[int64]*ChainConfig{
	137: { // Polygon 主网
		ChainID:           137,
		Exchange:          common.HexToAddress("0x4bFb41d5B3570DeFd03C39a9A4D8dE6Bd8B8982E"),
		Collateral:        common.HexToAddress("0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174"),
		ConditionalTokens: common.HexToAddress("0x4D97DCd97eC945f40cF65F87097ACe5EA0476045"),
		NegRiskExchange:   common.HexToAddress("0xC5d563A36AE78145C45a50134d48A1215220f80a"),
	},
	80002: { // Amoy 测试网
		ChainID:           80002,
		Exchange:          common.HexToAddress("0xdFE02Eb6733538f8Ea35D585af8DE5958AD99E40"),
		Collateral:        common.HexToAddress("0x9c4e1703476e875070ee25b56a58b008cfb8fa78"),
		ConditionalTokens: common.HexToAddress("0x69308FB512518e39F9b16112fA8d994F4e2Bf8bB"),
		NegRiskExchange:   common.HexToAddress("0xd91E80cF2E7be2e162c6513ceD06f1dD0dA35296"),
	},
}

// BaseWeb3Client Web3 基础客户端
type BaseWeb3Client struct {
	client        *ethclient.Client
	privateKey    *ecdsa.PrivateKey
	account       common.Address
	signatureType SignatureType
	chainID       int64
	config        *ChainConfig
	negRiskConfig *ChainConfig

	// 地址（根据签名类型不同）
	Address common.Address

	// 合约地址
	USDCAddress              common.Address
	ConditionalTokensAddress common.Address
	ExchangeAddress          common.Address
	NegRiskExchangeAddress   common.Address
	NegRiskAdapterAddress    common.Address
	ProxyFactoryAddress      common.Address
	SafeProxyFactoryAddress  common.Address
}

// NewBaseWeb3Client 创建基础 Web3 客户端
func NewBaseWeb3Client(
	privateKey string,
	signatureType SignatureType,
	chainID int64,
	rpcURL string,
) (*BaseWeb3Client, error) {
	if rpcURL == "" {
		rpcURL = DefaultPolygonRPC
	}

	// 连接到以太坊客户端
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Ethereum client: %w", err)
	}

	// 解析私钥
	privKey, err := crypto.HexToECDSA(stripHexPrefix(privateKey))
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// 获取账户地址
	publicKey := privKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("failed to get public key")
	}
	account := crypto.PubkeyToAddress(*publicKeyECDSA)

	// 获取链配置
	config, ok := chainConfigs[chainID]
	if !ok {
		return nil, fmt.Errorf("unsupported chain ID: %d", chainID)
	}

	// Neg risk 使用相同的配置但不同的交易所地址
	negRiskConfig := &ChainConfig{
		ChainID:           config.ChainID,
		Exchange:          config.NegRiskExchange,
		Collateral:        config.Collateral,
		ConditionalTokens: config.ConditionalTokens,
		NegRiskExchange:   config.NegRiskExchange,
	}

	c := &BaseWeb3Client{
		client:        client,
		privateKey:    privKey,
		account:       account,
		signatureType: signatureType,
		chainID:       chainID,
		config:        config,
		negRiskConfig: negRiskConfig,

		USDCAddress:              config.Collateral,
		ConditionalTokensAddress: config.ConditionalTokens,
		ExchangeAddress:          config.Exchange,
		NegRiskExchangeAddress:   config.NegRiskExchange,
		NegRiskAdapterAddress:    NegRiskAdapterAddress,
		ProxyFactoryAddress:      ProxyFactoryAddress,
		SafeProxyFactoryAddress:  SafeProxyFactoryAddress,
	}

	// 设置地址（根据签名类型）
	if err := c.setupAddress(); err != nil {
		return nil, err
	}

	return c, nil
}

// setupAddress 根据签名类型设置地址
func (c *BaseWeb3Client) setupAddress() error {
	switch c.signatureType {
	case SignatureTypeEOA:
		c.Address = c.account
	case SignatureTypePolyProxy:
		addr, err := c.GetPolyProxyAddress(c.account)
		if err != nil {
			return err
		}
		c.Address = addr
	case SignatureTypeSafe:
		addr, err := c.GetSafeProxyAddress(c.account)
		if err != nil {
			return err
		}
		c.Address = addr
	default:
		return fmt.Errorf("invalid signature type: %d", c.signatureType)
	}
	return nil
}

// GetBaseAddress 获取基础 EOA 地址
func (c *BaseWeb3Client) GetBaseAddress() common.Address {
	return c.account
}

// GetPolyProxyAddress 获取 Polymarket 代理地址
func (c *BaseWeb3Client) GetPolyProxyAddress(address common.Address) (common.Address, error) {
	// 调用 CTFExchange 合约的 getPolyProxyWalletAddress 方法
	data, err := CTFExchangeABI.Pack("getPolyProxyWalletAddress", address)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to pack call data: %w", err)
	}

	msg := ethereum.CallMsg{
		To:        &c.ExchangeAddress,
		Data:      data,
		GasFeeCap: callGasFeeCap,
		GasTipCap: callGasTipCap,
	}

	result, err := c.client.CallContract(context.Background(), msg, nil)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to call contract: %w", err)
	}

	var proxyAddress common.Address
	err = CTFExchangeABI.UnpackIntoInterface(&proxyAddress, "getPolyProxyWalletAddress", result)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to unpack result: %w", err)
	}

	return proxyAddress, nil
}

// GetSafeProxyAddress 获取 Safe 代理地址
func (c *BaseWeb3Client) GetSafeProxyAddress(address common.Address) (common.Address, error) {
	// 调用 SafeProxyFactory 合约的 computeProxyAddress 方法
	data, err := SafeProxyFactoryABI.Pack("computeProxyAddress", address)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to pack call data: %w", err)
	}

	msg := ethereum.CallMsg{
		To:        &c.SafeProxyFactoryAddress,
		Data:      data,
		GasFeeCap: callGasFeeCap,
		GasTipCap: callGasTipCap,
	}

	result, err := c.client.CallContract(context.Background(), msg, nil)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to call contract: %w", err)
	}

	var proxyAddress common.Address
	err = SafeProxyFactoryABI.UnpackIntoInterface(&proxyAddress, "computeProxyAddress", result)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to unpack result: %w", err)
	}

	return proxyAddress, nil
}

// GetPOLBalance 获取 POL 余额
func (c *BaseWeb3Client) GetPOLBalance() (*big.Float, error) {
	balance, err := c.client.BalanceAt(context.Background(), c.account, nil)
	if err != nil {
		return nil, err
	}

	// 转换为 POL（18 位小数）
	balanceFloat := new(big.Float).SetInt(balance)
	divisor := new(big.Float).SetInt(big.NewInt(1e18))
	result := new(big.Float).Quo(balanceFloat, divisor)

	return result, nil
}

// GetUSDCBalance 获取 USDC 余额
func (c *BaseWeb3Client) GetUSDCBalance(address common.Address) (*big.Float, error) {
	if address == (common.Address{}) {
		address = c.Address
	}

	// 调用 USDC 合约的 balanceOf 方法
	data, err := USDCABI.Pack("balanceOf", address)
	if err != nil {
		return nil, fmt.Errorf("failed to pack call data: %w", err)
	}

	msg := ethereum.CallMsg{
		To:        &c.USDCAddress,
		Data:      data,
		GasFeeCap: callGasFeeCap,
		GasTipCap: callGasTipCap,
	}

	result, err := c.client.CallContract(context.Background(), msg, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to call contract: %w", err)
	}

	var balance *big.Int
	err = USDCABI.UnpackIntoInterface(&balance, "balanceOf", result)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack result: %w", err)
	}

	// 转换为 USDC（6 位小数）
	balanceFloat := new(big.Float).SetInt(balance)
	divisor := new(big.Float).SetInt(big.NewInt(1e6))
	resultFloat := new(big.Float).Quo(balanceFloat, divisor)

	return resultFloat, nil
}

// GetTokenBalance 获取条件代币余额
func (c *BaseWeb3Client) GetTokenBalance(tokenID string, address common.Address) (*big.Float, error) {
	if address == (common.Address{}) {
		address = c.Address
	}

	tokenIDBig := new(big.Int)
	tokenIDBig.SetString(tokenID, 10)

	// 调用 ConditionalTokens 合约的 balanceOf 方法
	data, err := ConditionalTokensABI.Pack("balanceOf", address, tokenIDBig)
	if err != nil {
		return nil, fmt.Errorf("failed to pack call data: %w", err)
	}

	msg := ethereum.CallMsg{
		To:        &c.ConditionalTokensAddress,
		Data:      data,
		GasFeeCap: callGasFeeCap,
		GasTipCap: callGasTipCap,
	}

	result, err := c.client.CallContract(context.Background(), msg, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to call contract: %w", err)
	}

	var balance *big.Int
	err = ConditionalTokensABI.UnpackIntoInterface(&balance, "balanceOf", result)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack result: %w", err)
	}

	// 转换（6 位小数）
	balanceFloat := new(big.Float).SetInt(balance)
	divisor := new(big.Float).SetInt(big.NewInt(1e6))
	resultFloat := new(big.Float).Quo(balanceFloat, divisor)

	return resultFloat, nil
}

// GetTokenComplement 获取互补代币 ID
func (c *BaseWeb3Client) GetTokenComplement(tokenID string) (string, error) {
	tokenIDBig := new(big.Int)
	tokenIDBig.SetString(tokenID, 10)

	// 首先尝试 NegRiskExchange
	data, err := NegRiskExchangeABI.Pack("getComplement", tokenIDBig)
	if err != nil {
		return "", fmt.Errorf("failed to pack call data: %w", err)
	}

	msg := ethereum.CallMsg{
		To:        &c.NegRiskExchangeAddress,
		Data:      data,
		GasFeeCap: callGasFeeCap,
		GasTipCap: callGasTipCap,
	}

	result, err := c.client.CallContract(context.Background(), msg, nil)
	if err == nil && len(result) > 0 {
		var complement *big.Int
		err = NegRiskExchangeABI.UnpackIntoInterface(&complement, "getComplement", result)
		if err == nil && complement.Sign() > 0 {
			return complement.String(), nil
		}
	}

	// 回退到 CTFExchange
	data, err = CTFExchangeABI.Pack("getComplement", tokenIDBig)
	if err != nil {
		return "", fmt.Errorf("failed to pack call data: %w", err)
	}

	msg = ethereum.CallMsg{
		To:        &c.ExchangeAddress,
		Data:      data,
		GasFeeCap: callGasFeeCap,
		GasTipCap: callGasTipCap,
	}

	result, err = c.client.CallContract(context.Background(), msg, nil)
	if err != nil {
		return "", fmt.Errorf("failed to call contract: %w", err)
	}

	var complement *big.Int
	err = CTFExchangeABI.UnpackIntoInterface(&complement, "getComplement", result)
	if err != nil {
		return "", fmt.Errorf("failed to unpack result: %w", err)
	}

	return complement.String(), nil
}

// GetConditionIDNegRisk 获取 neg risk 市场的 condition ID
func (c *BaseWeb3Client) GetConditionIDNegRisk(questionID common.Hash) (common.Hash, error) {
	// 调用 NegRiskAdapter 合约的 getConditionId 方法
	data, err := NegRiskAdapterABI.Pack("getConditionId", questionID)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to pack call data: %w", err)
	}

	msg := ethereum.CallMsg{
		To:        &c.NegRiskAdapterAddress,
		Data:      data,
		GasFeeCap: callGasFeeCap,
		GasTipCap: callGasTipCap,
	}

	result, err := c.client.CallContract(context.Background(), msg, nil)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to call contract: %w", err)
	}

	var conditionID [32]byte
	err = NegRiskAdapterABI.UnpackIntoInterface(&conditionID, "getConditionId", result)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to unpack result: %w", err)
	}

	return common.BytesToHash(conditionID[:]), nil
}

// encodeUSDCApprove 编码 USDC 授权交易
func (c *BaseWeb3Client) encodeUSDCApprove(spender common.Address) ([]byte, error) {
	return USDCABI.Pack("approve", spender, MaxUint256())
}

// encodeConditionalTokensApprove 编码条件代币授权交易
func (c *BaseWeb3Client) encodeConditionalTokensApprove(spender common.Address) ([]byte, error) {
	return ConditionalTokensABI.Pack("setApprovalForAll", spender, true)
}

// encodeTransferUSDC 编码 USDC 转账交易
func (c *BaseWeb3Client) encodeTransferUSDC(to common.Address, amount *big.Int) ([]byte, error) {
	return USDCABI.Pack("transfer", to, amount)
}

// encodeTransferToken 编码条件代币转账交易
func (c *BaseWeb3Client) encodeTransferToken(tokenID string, to common.Address, amount *big.Int) ([]byte, error) {
	tokenIDBig := new(big.Int)
	tokenIDBig.SetString(tokenID, 10)
	return ConditionalTokensABI.Pack("safeTransferFrom", c.Address, to, tokenIDBig, amount, []byte{})
}

// encodeSplit 编码拆分仓位交易
func (c *BaseWeb3Client) encodeSplit(conditionID common.Hash, amount *big.Int) ([]byte, error) {
	return ConditionalTokensABI.Pack("splitPosition", c.USDCAddress, HashZero, conditionID, []*big.Int{big.NewInt(1), big.NewInt(2)}, amount)
}

// encodeMerge 编码合并仓位交易
func (c *BaseWeb3Client) encodeMerge(conditionID common.Hash, amount *big.Int) ([]byte, error) {
	return ConditionalTokensABI.Pack("mergePositions", c.USDCAddress, HashZero, conditionID, []*big.Int{big.NewInt(1), big.NewInt(2)}, amount)
}

// encodeRedeem 编码赎回仓位交易
func (c *BaseWeb3Client) encodeRedeem(conditionID common.Hash) ([]byte, error) {
	return ConditionalTokensABI.Pack("redeemPositions", c.USDCAddress, HashZero, conditionID, []*big.Int{big.NewInt(1), big.NewInt(2)})
}

// encodeRedeemNegRisk 编码 neg risk 赎回仓位交易
func (c *BaseWeb3Client) encodeRedeemNegRisk(conditionID common.Hash, amounts []*big.Int) ([]byte, error) {
	return NegRiskAdapterABI.Pack("redeemPositions", conditionID, amounts)
}

// encodeConvert 编码转换仓位交易
func (c *BaseWeb3Client) encodeConvert(negRiskMarketID common.Hash, indexSet *big.Int, amount *big.Int) ([]byte, error) {
	return NegRiskAdapterABI.Pack("convertPositions", negRiskMarketID, indexSet, amount)
}

// encodeProxy 编码代理交易
func (c *BaseWeb3Client) encodeProxy(proxyTxn interface{}) ([]byte, error) {
	return ProxyFactoryABI.Pack("proxy", []interface{}{proxyTxn})
}

// GetTransactionOpts 获取交易选项
func (c *BaseWeb3Client) GetTransactionOpts() (*bind.TransactOpts, error) {
	nonce, err := c.client.PendingNonceAt(context.Background(), c.account)
	if err != nil {
		return nil, err
	}

	gasPrice, err := c.client.SuggestGasPrice(context.Background())
	if err != nil {
		return nil, err
	}

	// 增加 5% 的 gas 价格
	gasPrice = new(big.Int).Mul(gasPrice, big.NewInt(105))
	gasPrice = new(big.Int).Div(gasPrice, big.NewInt(100))

	auth, err := bind.NewKeyedTransactorWithChainID(c.privateKey, big.NewInt(c.chainID))
	if err != nil {
		return nil, err
	}

	auth.Nonce = big.NewInt(int64(nonce))
	auth.Value = big.NewInt(0)
	auth.GasLimit = uint64(1000000)
	auth.GasPrice = gasPrice

	return auth, nil
}

// WaitForReceipt 等待交易收据
func (c *BaseWeb3Client) WaitForReceipt(txHash common.Hash) (*types.Receipt, error) {
	return bind.WaitMined(context.Background(), c.client, &types.Transaction{})
}

// Client 返回底层的 ethclient
func (c *BaseWeb3Client) Client() *ethclient.Client {
	return c.client
}

// PrivateKey 返回私钥
func (c *BaseWeb3Client) PrivateKey() *ecdsa.PrivateKey {
	return c.privateKey
}

// ChainID 返回链 ID
func (c *BaseWeb3Client) ChainID() int64 {
	return c.chainID
}

// SignatureType 返回签名类型
func (c *BaseWeb3Client) SignatureTypeValue() SignatureType {
	return c.signatureType
}

// stripHexPrefix 移除 0x 前缀
func stripHexPrefix(s string) string {
	if len(s) >= 2 && s[0:2] == "0x" {
		return s[2:]
	}
	return s
}

