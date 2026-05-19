package polymarket

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	obuilder "github.com/0xNetuser/Polymarket-golang/polymarket/order_builder"
	"github.com/0xNetuser/Polymarket-golang/polymarket/rfq"
	"github.com/ethereum/go-ethereum/common"
)

// ClobClient CLOB客户端
// 支持三种模式：
// 1. Level 0: 只需要host URL，可以访问公开端点
// 2. Level 1: 需要host, chain_id和私钥，可以访问L1认证端点
// 3. Level 2: 需要host, chain_id, 私钥和API凭证，可以访问所有端点
type ClobClient struct {
	host         string
	chainID      int
	signer       *Signer
	creds        *ApiCreds
	mode         int
	builder      *obuilder.OrderBuilder
	httpClient   *HTTPClient

	// V2 扩展
	sigType         int                                              // V2 signature type
	funder          string                                           // V2 funder 地址(POLY_1271 时为 deposit wallet)
	builderConfig   *BuilderConfig                                   // V2 全局 builder 配置
	builderV2Cache  map[string]*obuilder.ExchangeOrderBuilderV2     // key = contractAddress(lowercased)
	tokenCondition  map[string]string                                // tokenID -> conditionID
	marketDetails   map[string]*MarketDetails                        // conditionID -> details
	feeInfos        map[string]*FeeInfo                              // tokenID -> {rate, exponent}
	builderFeeRates map[string]*BuilderFeeRate                       // builderCode -> rates
	cachedVersion   *int                                             // 服务端订单版本(1 或 2)
	feeSlippage     float64                                          // 平台费率上浮百分比 (0 或 [1,100])

	// 本地缓存(V1)
	tickSizes map[string]TickSize
	negRisk   map[string]bool
	feeRates  map[string]int

	// RFQ客户端
	rfq *rfq.RfqClient

	mu sync.RWMutex
}

// NewClobClient 创建新的CLOB客户端
// host: CLOB API端点（如 "https://clob.polymarket.com"）
// chainID: 链ID（137 for Polygon, 80002 for Amoy）
// privateKey: 私钥（十六进制字符串，可选）
// creds: API凭证（可选）
// signatureType: 签名类型（0=EOA, 1=Email/Magic, 2=Browser proxy，可选）
// funder: 资金持有者地址（用于代理钱包，可选）
func NewClobClient(host string, chainID int, privateKey string, creds *ApiCreds, signatureType *int, funder string) (*ClobClient, error) {
	// 移除host末尾的斜杠
	if strings.HasSuffix(host, "/") {
		host = host[:len(host)-1]
	}

	client := &ClobClient{
		host:            host,
		chainID:         chainID,
		creds:           creds,
		httpClient:      NewHTTPClient(host),
		tickSizes:       make(map[string]TickSize),
		negRisk:         make(map[string]bool),
		feeRates:        make(map[string]int),
		builderV2Cache:  make(map[string]*obuilder.ExchangeOrderBuilderV2),
		tokenCondition:  make(map[string]string),
		marketDetails:   make(map[string]*MarketDetails),
		feeInfos:        make(map[string]*FeeInfo),
		builderFeeRates: make(map[string]*BuilderFeeRate),
	}

	// 创建签名器（如果提供了私钥）
	if privateKey != "" {
		signer, err := NewSigner(privateKey, chainID)
		if err != nil {
			return nil, fmt.Errorf("failed to create signer: %w", err)
		}
		client.signer = signer

		// 创建订单构建器
		sigType := SigTypeEOA
		if signatureType != nil {
			sigType = *signatureType
		}

		funderAddr := signer.Address()
		if funder != "" {
			// 防止非法字符串被 common.HexToAddress 静默截断为 0x000...000:
			// 进而签出一笔"出资人=零地址"的废订单。POLY_1271 路径下尤其严重,
			// 因为 funder 同时被用作 maker + signer + Deposit Wallet 验证目标。
			if !common.IsHexAddress(funder) {
				return nil, fmt.Errorf("invalid funder address: %q (must be a hex Ethereum address)", funder)
			}
			funderAddr = funder
		}

		client.sigType = sigType
		client.funder = funderAddr

		builder, err := obuilder.NewOrderBuilder(signer, sigType, funderAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to create order builder: %w", err)
		}
		client.builder = builder
	}

	// 确定客户端模式
	client.mode = client.getClientMode()

	// 创建RFQ客户端
	client.rfq = rfq.NewRfqClient(client)

	return client, nil
}

// getClientMode 获取客户端模式
func (c *ClobClient) getClientMode() int {
	if c.signer == nil {
		return L0
	}
	if c.creds == nil {
		return L1
	}
	return L2
}

// GetAddress 返回签名器的地址
func (c *ClobClient) GetAddress() string {
	if c.signer == nil {
		return ""
	}
	return c.signer.Address()
}

// GetCollateralAddress 返回抵押品代币地址
func (c *ClobClient) GetCollateralAddress() string {
	config := getContractConfig(c.chainID, false)
	if config != nil {
		return config.Collateral
	}
	return ""
}

// GetConditionalAddress 返回条件代币地址
func (c *ClobClient) GetConditionalAddress() string {
	config := getContractConfig(c.chainID, false)
	if config != nil {
		return config.ConditionalTokens
	}
	return ""
}

// GetExchangeAddress 返回交易所地址
func (c *ClobClient) GetExchangeAddress(negRisk bool) string {
	config := getContractConfig(c.chainID, negRisk)
	if config != nil {
		return config.Exchange
	}
	return ""
}

// SetAPICreds 设置API凭证
func (c *ClobClient) SetAPICreds(creds *ApiCreds) {
	c.creds = creds
	c.mode = c.getClientMode()
}

// assertLevel1Auth 断言需要L1认证
func (c *ClobClient) assertLevel1Auth() error {
	if c.mode < L1 {
		return fmt.Errorf(L1AuthUnavailable)
	}
	return nil
}

// assertLevel2Auth 断言需要L2认证
func (c *ClobClient) assertLevel2Auth() error {
	if c.mode < L2 {
		return fmt.Errorf(L2AuthUnavailable)
	}
	return nil
}

// AssertLevel2Auth 断言需要L2认证（导出方法，供RFQ客户端使用）
func (c *ClobClient) AssertLevel2Auth() error {
	return c.assertLevel2Auth()
}

// GetSigner 获取签名器（供RFQ客户端使用）
func (c *ClobClient) GetSigner() *Signer {
	return c.signer
}

// GetCreds 获取API凭证（供RFQ客户端使用）
func (c *ClobClient) GetCreds() *ApiCreds {
	return c.creds
}

// GetHTTPClient 获取HTTP客户端（供RFQ客户端使用）
func (c *ClobClient) GetHTTPClient() rfq.HTTPClientInterface {
	return c.httpClient
}

// GetRFQ 获取RFQ客户端
func (c *ClobClient) GetRFQ() *rfq.RfqClient {
	return c.rfq
}

// CreateLevel2HeadersInternal 创建L2认证头（供RFQ客户端使用，避免循环导入）
func (c *ClobClient) CreateLevel2HeadersInternal(method, path string, body interface{}) (map[string]string, error) {
	var bodyStr string
	if body != nil {
		bodyJSON, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		bodyStr = string(bodyJSON)
	}
	
	requestArgs := &RequestArgs{
		Method:        method,
		RequestPath:   path,
		Body:          body,
		SerializedBody: &bodyStr,
	}

	return CreateLevel2Headers(c.signer, c.creds, requestArgs)
}

// GetHost 获取host（供RFQ客户端使用）
func (c *ClobClient) GetHost() string {
	return c.host
}

// GetAPICreds 获取API Key（供RFQ客户端使用）
func (c *ClobClient) GetAPICreds() string {
	if c.creds != nil {
		return c.creds.APIKey
	}
	return ""
}

// CreateOrderForRFQ 为 RFQ 创建签名订单(供 rfq 子包使用)。
//
// V2 迁移完成后,普通下单走 CreateOrderV2;RFQ 当前仍构造 V1 订单
// (Polymarket V2 服务端的 RFQ 子系统仍接受 V1 订单格式),所以这里走的是
// 内部 createOrderV1ForRFQ 路径。
func (c *ClobClient) CreateOrderForRFQ(args *rfq.OrderCreationArgs) (*rfq.SignedOrderData, error) {
	signedOrder, err := c.createOrderV1ForRFQ(&rfqOrderArgsV1{
		TokenID:    args.TokenID,
		Price:      args.Price,
		Size:       args.Size,
		Side:       args.Side,
		Expiration: int64(args.Expiration),
	})
	if err != nil {
		return nil, err
	}

	// 将 side 转换为字符串
	sideStr := "BUY"
	if signedOrder.Side.Int64() == 1 {
		sideStr = "SELL"
	}

	// 转换为 RFQ 需要的格式
	return &rfq.SignedOrderData{
		Salt:          signedOrder.Salt.Int64(),
		Maker:         signedOrder.Maker.Hex(),
		Signer:        signedOrder.Signer.Hex(),
		Taker:         signedOrder.Taker.Hex(),
		TokenID:       signedOrder.TokenId.String(),
		MakerAmount:   signedOrder.MakerAmount.String(),
		TakerAmount:   signedOrder.TakerAmount.String(),
		Expiration:    signedOrder.Expiration.String(),
		Nonce:         signedOrder.Nonce.String(),
		FeeRateBps:    signedOrder.FeeRateBps.String(),
		Side:          sideStr,
		SignatureType: int(signedOrder.SignatureType.Int64()),
		Signature:     "0x" + common.Bytes2Hex(signedOrder.Signature),
	}, nil
}

