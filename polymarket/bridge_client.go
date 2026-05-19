// Bridge API client(法币 / 跨链入金 + 出金)
//
// 文档:https://docs.polymarket.com/api-reference/bridge/*
//
// Bridge 是独立服务,base URL = https://bridge.polymarket.com,**全部端点无需认证**。
// 用途:把任意链上的资产(ETH/Solana/BTC/各种 EVM token)bridge 成 Polygon 上的
// pUSD 进你的 Polymarket 钱包。
//
// 这块 SDK 独立成 BridgeClient,跟主 ClobClient 解耦 —— bridge 不需要 EOA 也不
// 需要 builder key,只要 funder 地址就能查 quote / 拉 deposit 地址 / 轮询状态。
package polymarket

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// BridgeBaseURL Polymarket Bridge API host。
const BridgeBaseURL = "https://bridge.polymarket.com"

// Bridge endpoint paths
const (
	BridgePathSupportedAssets = "/supported-assets"
	BridgePathQuote           = "/quote"
	BridgePathStatus          = "/status/" // + {address}
	BridgePathDeposit         = "/deposit"
	BridgePathWithdraw        = "/withdraw"
)

// ============================================================
//  Schema types(对齐 doc)
// ============================================================

// BridgeToken 单一 token 描述。
type BridgeToken struct {
	Name     string `json:"name"`
	Symbol   string `json:"symbol"`
	Address  string `json:"address"`
	Decimals int    `json:"decimals"`
}

// BridgeSupportedAsset 一个链上的某种支持的 token。
type BridgeSupportedAsset struct {
	ChainID        string      `json:"chainId"`
	ChainName      string      `json:"chainName"`
	Token          BridgeToken `json:"token"`
	MinCheckoutUsd float64     `json:"minCheckoutUsd"`
}

// BridgeSupportedAssetsResponse GET /supported-assets 响应。
type BridgeSupportedAssetsResponse struct {
	SupportedAssets []BridgeSupportedAsset `json:"supportedAssets"`
}

// BridgeQuoteRequest POST /quote 请求 body。
//
// FromAmountBaseUnit 必填,字符串形式,**最小单位**(decimals 取决于 token)。
type BridgeQuoteRequest struct {
	FromAmountBaseUnit string `json:"fromAmountBaseUnit"`
	FromChainID        string `json:"fromChainId"`
	FromTokenAddress   string `json:"fromTokenAddress"`
	RecipientAddress   string `json:"recipientAddress"`
	ToChainID          string `json:"toChainId"`
	ToTokenAddress     string `json:"toTokenAddress"`
}

// BridgeFeeBreakdown 报价里的明细费率。
type BridgeFeeBreakdown struct {
	AppFeeLabel     string  `json:"appFeeLabel"`
	AppFeePercent   float64 `json:"appFeePercent"`
	AppFeeUsd       float64 `json:"appFeeUsd"`
	FillCostPercent float64 `json:"fillCostPercent"`
	FillCostUsd     float64 `json:"fillCostUsd"`
	GasUsd          float64 `json:"gasUsd"`
	MaxSlippage     float64 `json:"maxSlippage"`
	MinReceived     float64 `json:"minReceived"`
	SwapImpact      float64 `json:"swapImpact"`
	SwapImpactUsd   float64 `json:"swapImpactUsd"`
	TotalImpact     float64 `json:"totalImpact"`
	TotalImpactUsd  float64 `json:"totalImpactUsd"`
}

// BridgeQuoteResponse POST /quote 响应。
type BridgeQuoteResponse struct {
	EstCheckoutTimeMs   int                `json:"estCheckoutTimeMs"`
	EstFeeBreakdown     BridgeFeeBreakdown `json:"estFeeBreakdown"`
	EstInputUsd         float64            `json:"estInputUsd"`
	EstOutputUsd        float64            `json:"estOutputUsd"`
	EstToTokenBaseUnit  string             `json:"estToTokenBaseUnit"`
	QuoteID             string             `json:"quoteId"`
}

// BridgeDepositRequest POST /deposit 请求 body。
type BridgeDepositRequest struct {
	Address string `json:"address"` // 0x-prefixed 40 hex(Polymarket proxy wallet)
}

// BridgeDepositAddresses 多链 deposit / withdraw 接收地址组。
type BridgeDepositAddresses struct {
	EVM string `json:"evm"`
	SVM string `json:"svm"`
	BTC string `json:"btc"`
}

// BridgeDepositResponse POST /deposit 与 POST /withdraw 共用的响应结构。
type BridgeDepositResponse struct {
	Address BridgeDepositAddresses `json:"address"`
	Note    string                 `json:"note"`
}

// BridgeWithdrawRequest POST /withdraw 请求 body。
type BridgeWithdrawRequest struct {
	Address        string `json:"address"`        // 源 Polymarket Polygon proxy wallet 地址
	ToChainID      string `json:"toChainId"`      // 目标链 ID
	ToTokenAddress string `json:"toTokenAddress"` // 目标 token
	RecipientAddr  string `json:"recipientAddr"`  // 目标接收地址
}

// BridgeTransactionStatus 单笔 bridge tx 的状态枚举。
type BridgeTransactionStatus string

const (
	BridgeStatusDepositDetected   BridgeTransactionStatus = "DEPOSIT_DETECTED"
	BridgeStatusProcessing        BridgeTransactionStatus = "PROCESSING"
	BridgeStatusOriginTxConfirmed BridgeTransactionStatus = "ORIGIN_TX_CONFIRMED"
	BridgeStatusSubmitted         BridgeTransactionStatus = "SUBMITTED"
	BridgeStatusCompleted         BridgeTransactionStatus = "COMPLETED"
	BridgeStatusFailed            BridgeTransactionStatus = "FAILED"
)

// BridgeTransaction GET /status/{address} 返回的单笔 tx。
type BridgeTransaction struct {
	FromChainID        string                  `json:"fromChainId"`
	FromTokenAddress   string                  `json:"fromTokenAddress"`
	FromAmountBaseUnit string                  `json:"fromAmountBaseUnit"`
	ToChainID          string                  `json:"toChainId"`
	ToTokenAddress     string                  `json:"toTokenAddress"`
	Status             BridgeTransactionStatus `json:"status"`
	TxHash             string                  `json:"txHash,omitempty"`      // 只有 COMPLETED 状态才有
	CreatedTimeMs      int64                   `json:"createdTimeMs,omitempty"` // DEPOSIT_DETECTED 时 absent
}

// BridgeStatusResponse GET /status/{address} 响应。
type BridgeStatusResponse struct {
	Transactions []BridgeTransaction `json:"transactions"`
}

// ============================================================
//  BridgeClient
// ============================================================

// BridgeClient Polymarket Bridge API 客户端。
// 不需要 EOA / 不需要 builder key,只有一个 base URL + http client。
type BridgeClient struct {
	baseURL string
	http    *http.Client
}

// NewBridgeClient 创建 Bridge 客户端(用默认 BridgeBaseURL)。
func NewBridgeClient() *BridgeClient {
	return &BridgeClient{
		baseURL: BridgeBaseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// WithBaseURL 用于测试时指定 mock URL。
func (b *BridgeClient) WithBaseURL(u string) *BridgeClient {
	b.baseURL = u
	return b
}

// WithHTTPClient 注入自定义 http client(超时 / TLS 等)。
func (b *BridgeClient) WithHTTPClient(h *http.Client) *BridgeClient {
	b.http = h
	return b
}

// GetSupportedAssets 列出所有可 bridge 的链 + token + 最低门槛。
func (b *BridgeClient) GetSupportedAssets() (*BridgeSupportedAssetsResponse, error) {
	var out BridgeSupportedAssetsResponse
	if err := b.do("GET", BridgePathSupportedAssets, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetQuote 估算 bridge fee / 时间 / 收到的金额。
func (b *BridgeClient) GetQuote(req *BridgeQuoteRequest) (*BridgeQuoteResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("BridgeQuoteRequest required")
	}
	if req.FromAmountBaseUnit == "" || req.FromChainID == "" || req.FromTokenAddress == "" ||
		req.RecipientAddress == "" || req.ToChainID == "" || req.ToTokenAddress == "" {
		return nil, fmt.Errorf("all BridgeQuoteRequest fields are required")
	}
	var out BridgeQuoteResponse
	if err := b.do("POST", BridgePathQuote, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateDepositAddresses 给指定 Polymarket 钱包申请多链 deposit 地址。
// 向返回的某条链上的某个地址转账,资产会自动 bridge 成 pUSD 入你的 wallet。
func (b *BridgeClient) CreateDepositAddresses(walletAddress string) (*BridgeDepositResponse, error) {
	if walletAddress == "" {
		return nil, fmt.Errorf("walletAddress required")
	}
	var out BridgeDepositResponse
	if err := b.do("POST", BridgePathDeposit, &BridgeDepositRequest{Address: walletAddress}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateWithdrawAddresses 申请出金到指定目标链的接收地址。
func (b *BridgeClient) CreateWithdrawAddresses(req *BridgeWithdrawRequest) (*BridgeDepositResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("BridgeWithdrawRequest required")
	}
	if req.Address == "" || req.ToChainID == "" || req.ToTokenAddress == "" || req.RecipientAddr == "" {
		return nil, fmt.Errorf("all BridgeWithdrawRequest fields are required")
	}
	var out BridgeDepositResponse
	if err := b.do("POST", BridgePathWithdraw, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetStatus 轮询某个 deposit / withdraw 地址下的所有 bridge tx 状态。
// address 是 CreateDepositAddresses / CreateWithdrawAddresses 返回的任意一个 EVM/SVM/BTC 地址。
func (b *BridgeClient) GetStatus(address string) (*BridgeStatusResponse, error) {
	if address == "" {
		return nil, fmt.Errorf("address required")
	}
	var out BridgeStatusResponse
	if err := b.do("GET", BridgePathStatus+address, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// do 共用 HTTP 调用 —— 简化错误处理 + body 解码。
func (b *BridgeClient) do(method, path string, body interface{}, out interface{}) error {
	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reqBody = strings.NewReader(string(buf))
	}
	req, err := http.NewRequest(method, b.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := b.http.Do(req)
	if err != nil {
		return fmt.Errorf("bridge %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		// 服务端常返回 {"error": "..."},把它带上方便排查
		var errEnv struct{ Error string `json:"error"` }
		_ = json.Unmarshal(data, &errEnv)
		if errEnv.Error != "" {
			return fmt.Errorf("bridge %s %s -> %d: %s", method, path, resp.StatusCode, errEnv.Error)
		}
		return fmt.Errorf("bridge %s %s -> %d: %s", method, path, resp.StatusCode, string(data))
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode bridge response: %w (body=%s)", err, string(data))
		}
	}
	return nil
}
