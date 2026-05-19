// builder_api_key 演示创建 / 查询 / 撤销 V2 builder API key 和拉取 builder 费率。
//
// V2 引入 builder code (bytes32) 用于第三方接入方做返佣。流程:
//   1. 用 L2 API key 调用 POST /auth/builder-api-key 注册一个 builder API key
//   2. 后续创建订单时把 builder code 填进 OrderArgsV2.BuilderCode 或
//      ClobClient.SetBuilderConfig()
//   3. 也可以查 /fees/builder-fees/{code} 看 maker/taker 费率
//
// 环境变量:
//   PRIVATE_KEY, CLOB_API_KEY/CLOB_SECRET/CLOB_PASSPHRASE — 已有 L2 凭证
//   BUILDER_CODE — bytes32 builder code(0x… 64 个 hex 字符)
//   ACTION — create | list | revoke | rate(默认 list)
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/0xNetuser/Polymarket-golang/polymarket"
)

func main() {
	pk := requireEnv("PRIVATE_KEY")
	creds := &polymarket.ApiCreds{
		APIKey:        requireEnv("CLOB_API_KEY"),
		APISecret:     requireEnv("CLOB_SECRET"),
		APIPassphrase: requireEnv("CLOB_PASSPHRASE"),
	}
	host := orDefault("CLOB_HOST", "https://clob.polymarket.com")
	client, err := polymarket.NewClobClient(host, 137, pk, creds, nil, "")
	if err != nil {
		log.Fatalf("new client: %v", err)
	}

	action := orDefault("ACTION", "list")
	switch action {
	case "create":
		// V2 不需要在 body 里传 builder_code —— 服务端按 L2 凭证关联。
		// 想让后续订单自动带上 builder code,用 SetBuilderConfig。
		//
		// 这里演示 typed 便捷封装。要 raw JSON 用 CreateBuilderAPIKeyCreds()。
		creds, err := client.CreateBuilderAPIKey()
		if err != nil {
			log.Fatalf("create builder api key: %v", err)
		}
		fmt.Println("Builder credentials (保存好 secret/passphrase,不可恢复):")
		dump(creds)
	case "list":
		resp, err := client.GetBuilderAPIKeysList()
		if err != nil {
			log.Fatalf("list builder api keys: %v", err)
		}
		dump(resp)
	case "revoke":
		resp, err := client.RevokeBuilderAPIKeyCreds()
		if err != nil {
			log.Fatalf("revoke: %v", err)
		}
		dump(resp)
	case "rate":
		bc := requireEnv("BUILDER_CODE")
		rate, err := client.GetBuilderFeeRate(bc)
		if err != nil {
			log.Fatalf("get builder fee rate: %v", err)
		}
		dump(rate)
	default:
		log.Fatalf("unknown ACTION: %s (expected create/list/revoke/rate)", action)
	}
}

func requireEnv(k string) string {
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

func dump(v interface{}) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Printf("%+v\n", v)
		return
	}
	fmt.Println(string(b))
}
