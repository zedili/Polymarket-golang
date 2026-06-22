// place_order_v2 演示如何使用 V2 (CTF Exchange V2) 接口下单。
//
// V2 与 V1 的关键差异:
//   - 订单使用全新合约 exchange_v2 / neg_risk_exchange_v2
//   - 订单字段:移除 taker/nonce/feeRateBps,新增 timestamp/metadata/builder
//   - SDK 在下单前会自动调用 /version 探测服务器期望的版本,失败时自动重试
//
// 环境变量:
//
//	PRIVATE_KEY      EOA 私钥(必填)
//	TOKEN_ID         条件代币 ID(必填)
//	PRICE            限价(默认 0.5)
//	SIZE             份额(默认 5)
//	SIDE             BUY / SELL(默认 BUY)
//	BUILDER_CODE     bytes32 builder code(可选)
//	ORDER_TYPE       GTC / GTD / FOK / FAK(默认 GTC)
//	CHAIN_ID         137 / 80002(默认 137)
//	CLOB_HOST        默认 https://clob.polymarket.com
//	CLOB_API_KEY/CLOB_SECRET/CLOB_PASSPHRASE  L2 凭证(可选,缺失会自动 derive)
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/0xNetuser/Polymarket-golang/polymarket"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	pk := os.Getenv("PRIVATE_KEY")
	if pk == "" {
		log.Fatal("PRIVATE_KEY is required")
	}
	tokenID := os.Getenv("TOKEN_ID")
	if tokenID == "" {
		log.Fatal("TOKEN_ID is required")
	}

	host := envOr("CLOB_HOST", "https://clob.polymarket.com")
	chainID, _ := strconv.Atoi(envOr("CHAIN_ID", "137"))
	price, _ := strconv.ParseFloat(envOr("PRICE", "0.5"), 64)
	size, _ := strconv.ParseFloat(envOr("SIZE", "5"), 64)
	side := envOr("SIDE", "BUY")
	orderType := polymarket.OrderType(envOr("ORDER_TYPE", "GTC"))

	// 1. 创建客户端(默认 sigType=0 EOA)
	client, err := polymarket.NewClobClient(host, chainID, pk, "", nil, nil, "")
	if err != nil {
		log.Fatalf("new client: %v", err)
	}
	fmt.Printf("Wallet: %s\n", client.GetAddress())
	fmt.Printf("Chain : %d\n", chainID)

	// 2. 获取 L2 凭证(优先环境变量,否则 derive)
	creds := readOrDeriveCreds(client)
	client.SetAPICreds(creds)
	fmt.Println("L2 credentials ready")

	// 3. 探测服务器版本(GetVersion 失败时默认返回 V2,与 Python 一致)
	fmt.Printf("Server reports order version: %d\n", client.GetVersion())

	// 4. 构造 V2 限价单
	args := &polymarket.OrderArgsV2{
		TokenID:     tokenID,
		Price:       price,
		Size:        size,
		Side:        side,
		Expiration:  0,
		BuilderCode: envOr("BUILDER_CODE", ""),
	}
	fmt.Printf("\nPlacing V2 %s order: price=%.4f size=%.4f token=%s type=%s\n",
		side, price, size, tokenID, orderType)

	result, err := client.CreateAndPostOrderV2(args, nil, orderType, false, false)
	if err != nil {
		log.Fatalf("create+post v2: %v", err)
	}

	payloadJSON, _ := json.MarshalIndent(result.Payload, "", "  ")
	fmt.Println("\n--- Signed order payload ---")
	fmt.Println(string(payloadJSON))

	respJSON, _ := json.MarshalIndent(result.Response, "", "  ")
	fmt.Println("\n--- Server response ---")
	fmt.Println(string(respJSON))
}

func readOrDeriveCreds(c *polymarket.ClobClient) *polymarket.ApiCreds {
	if k, s, p := os.Getenv("CLOB_API_KEY"), os.Getenv("CLOB_SECRET"), os.Getenv("CLOB_PASSPHRASE"); k != "" && s != "" && p != "" {
		return &polymarket.ApiCreds{APIKey: k, APISecret: s, APIPassphrase: p}
	}
	nonce := 0
	creds, err := c.CreateOrDeriveAPIKey(&nonce)
	if err != nil {
		log.Fatalf("derive API key: %v", err)
	}
	return creds
}
