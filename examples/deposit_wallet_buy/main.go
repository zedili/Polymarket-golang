// deposit_wallet_buy 演示如何使用 V2 POLY_1271 (Deposit Wallet) 签名下单。
//
// Deposit Wallet 是 Polymarket V2 推出的智能合约钱包,链上验证签名走
// EIP-1271 + Solady TypedDataSign。要求:
//  1. EOA 已部署 Deposit Wallet 合约(链上动作,本示例不包含)
//  2. Deposit Wallet 已充值 USDC 并完成 CTF / Exchange approval
//  3. EOA 仍然持有签名权(本程序用 PRIVATE_KEY 签,signer 字段会指向 Deposit Wallet)
//
// 环境变量:
//
//	PRIVATE_KEY      EOA 私钥
//	DEPOSIT_WALLET   Deposit Wallet 合约地址(=funder)
//	TOKEN_ID         条件代币 ID
//	PRICE / SIZE     价/量
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/0xNetuser/Polymarket-golang/polymarket"
)

func main() {
	pk := mustEnv("PRIVATE_KEY")
	deposit := mustEnv("DEPOSIT_WALLET")
	tokenID := mustEnv("TOKEN_ID")
	chainID, _ := strconv.Atoi(orDefault("CHAIN_ID", "137"))
	price, _ := strconv.ParseFloat(orDefault("PRICE", "0.5"), 64)
	size, _ := strconv.ParseFloat(orDefault("SIZE", "100"), 64)
	side := orDefault("SIDE", "BUY")
	host := orDefault("CLOB_HOST", "https://clob.polymarket.com")

	// signatureType=3 表示 POLY_1271, funder = deposit wallet 合约地址
	sigType := polymarket.SigTypePoly1271
	client, err := polymarket.NewClobClient(host, chainID, pk, "", nil, &sigType, deposit)
	if err != nil {
		log.Fatalf("new client: %v", err)
	}
	fmt.Printf("EOA signer    : %s\n", client.GetAddress())
	fmt.Printf("Deposit wallet: %s\n", deposit)

	// L2 凭证(deposit wallet 需要预先 derive 过 API key)
	nonce := 0
	creds, err := client.CreateOrDeriveAPIKey(&nonce)
	if err != nil {
		log.Fatalf("derive api key: %v", err)
	}
	client.SetAPICreds(creds)

	args := &polymarket.OrderArgsV2{
		TokenID: tokenID,
		Price:   price,
		Size:    size,
		Side:    side,
	}
	resp, err := client.CreateAndPostOrderV2(args, nil, polymarket.OrderTypeGTC, false, false)
	if err != nil {
		log.Fatalf("place v2 order: %v", err)
	}
	out, _ := json.MarshalIndent(resp, "", "  ")
	fmt.Println(string(out))
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("%s is required", k)
	}
	return v
}

func orDefault(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
