# Polymarket Go SDK

[English](README.md) | [中文](README_zh.md)

Polymarket CLOB(中央限价订单簿)API 的 Go 语言 SDK,同时对齐官方 [py-clob-client](https://github.com/Polymarket/py-clob-client) (V1, 2026-05 起已归档)与 [py-clob-client-v2](https://github.com/Polymarket/py-clob-client-v2) (V2, 现行版本)。

Follow at X:  @netu5er

## 特性

- ✅ **V2-native** —— 对齐 `py-clob-client-v2`(V1 已于 2026-05 归档)
- ✅ **CTF Exchange V2** 订单签名(EIP-712 EOA + EIP-1271 Solady 包装,支持 Deposit Wallet)
- ✅ **自动版本协商** —— `/version` 缓存 + 遇到 `order_version_mismatch` 时自动重试
- ✅ **Builder code / Builder API key** —— 第三方接入返佣
- ✅ **三种认证级别**:L0、L1、L2(L2 HMAC 已对齐 V2 的预序列化 body 规范)
- ✅ **完整订单管理**:限价/市价、PostOnly、GTC/GTD/FOK/FAK、批量
- ✅ **RFQ**:报价请求流程
- ✅ **WebSocket** —— `/ws/market` + `/ws/user`,强类型 dispatcher、读超时、可重连 sentinel
- ✅ **Bridge** —— 跨链入金(`bridge.polymarket.com`)
- ✅ **Rewards / Rebates** —— maker 收益 / market reward 配置全套
- ✅ **链上 gasless** —— split / merge / redeem / convert / wrap USDC.e→pUSD
- ✅ **强类型** + 以 py-clob-client-v2 的 golden signature 做了 byte-for-byte 测试

## v0.3.0 新功能

完整 V2 迁移 + 四组新模块。下面是简单用法,完整变更看 [CHANGELOG.md](CHANGELOG.md)。

### WebSocket(实时盘口 + 用户事件)

```go
// market 频道(无需认证)
mc, _ := polymarket.NewMarketWSClient(
    []string{tokenID}, true, // custom_feature_enabled=true 同时推 new_market 事件
    &polymarket.WSHandler{
        OnBook:           func(e polymarket.WSBookEvent)         { /* 完整快照 */ },
        OnPriceChange:    func(e polymarket.WSPriceChangeEvent)  { /* 价位 delta */ },
        OnLastTradePrice: func(e polymarket.WSLastTradePriceEvent){ /* 成交 */ },
    },
)
go mc.Run(ctx) // 阻塞,ctx 取消 / ErrWSServerClose / ErrWSReadTimeout 才返回

// user 频道(L2 认证 —— 必须用 EOA 派生 key,Builder key 会被服务端立刻 close)
uc, _ := polymarket.NewUserWSClient(derivedCreds, []string{conditionID}, &polymarket.WSHandler{
    OnOrder: func(e polymarket.WSOrderEvent) { /* PLACEMENT/UPDATE/CANCELLATION */ },
    OnTrade: func(e polymarket.WSTradeEvent) { /* MATCHED/MINED/CONFIRMED */ },
})
go uc.Run(ctx)
```

读超时(3.5×ping)每帧自动重置,半开 TCP 不再静默冻死;decoder 错误走
`HandleUnknown("<event_type>:decode_error", raw)`,服务端 schema 漂移不会丢用户事件。

### USDC.e → pUSD wrap(CollateralOnramp)

V1 USDC.e condition redeem 之后,proxy 钱包里多出来的 USDC.e 在 Polymarket UI 上显示
"Pending deposit",SDK 一次 batch 就能 gasless 转成 pUSD:

```go
wc, _ := web3.NewPolymarketGaslessWeb3Client(pk, web3.SignatureTypePolyProxy,
    builderCreds, 137, rpcURL)

// approve(USDC.e → Onramp) + Onramp.wrap(USDC.e, recipient, amount) 一次中继
receipt, _ := wc.WrapUSDCeToPUSD(2.0, common.HexToAddress(funder))
```

### Bridge(跨链入金)

```go
b := polymarket.NewBridgeClient() // 独立 host:bridge.polymarket.com

assets, _ := b.GetSupportedAssets()                  // 209 个跨链 token
dep, _    := b.CreateDepositAddresses(walletAddress) // 返回 EVM/SVM/BTC 地址
quote, _  := b.GetQuote(&polymarket.BridgeQuoteRequest{...})
status, _ := b.GetStatus(dep.Address.EVM)            // 轮询进度
```

### Rewards / Rebates

```go
// 公开 —— 当前 reward 配置
all, _ := client.GetRewardsMarketsCurrent(nil, "")

// L2 —— 今天我的收益
earnings, _ := client.GetTotalEarningsForUserForDay(&polymarket.RewardsUserQuery{
    Date: "2026-05-19", MakerAddress: funder,
})
```

### Data API + Gamma API

```go
da := polymarket.NewDataAPIClient()
redeemable := true
positions, _ := da.GetPositions(&polymarket.PositionsQuery{
    User: walletAddress, Redeemable: &redeemable,
})

ga := polymarket.NewGammaAPIClient()
active, limit := true, 50
markets, _ := ga.ListMarkets(&polymarket.GammaMarketsQuery{Active: &active, Limit: &limit})
```

## V2 快速开始

```go
client, _ := polymarket.NewClobClient("https://clob.polymarket.com", 137, pk, creds, nil, "")

// 可选:全局注册 builder code
client.SetBuilderConfig(&polymarket.BuilderConfig{
    BuilderCode: "0xabc...0001", // bytes32
})

// V2 限价买单 (timestamp / metadata / builder 由 SDK 自动填充)
resp, err := client.CreateAndPostOrderV2(
    &polymarket.OrderArgsV2{
        TokenID:    "713210...",
        Price:      0.4,
        Size:       100,
        Side:       polymarket.BUY,
        Expiration: 0,
    },
    nil,                       // PartialCreateOrderOptions
    polymarket.OrderTypeGTC,
    false, false,              // postOnly, deferExec
)
```

使用 Polymarket Deposit Wallet (智能合约钱包, EIP-1271 / Solady):

```go
sig := polymarket.SigTypePoly1271
client, _ := polymarket.NewClobClient(host, chainID, eoaPrivateKey, creds, &sig, depositWalletAddress)
```

更多示例请见 `examples/place_order_v2/`、`examples/deposit_wallet_buy/`、`examples/builder_api_key/`。

## 安装

```bash
go get github.com/0xNetuser/Polymarket-golang
```

## 快速开始

### Level 0 — 只读

```go
client, _ := polymarket.NewClobClient("https://clob.polymarket.com", 137, "", nil, nil, "")
book, _ := client.GetOrderBook("token-id")
fmt.Printf("server version: %d\n", client.GetVersion())
```

### Level 1 — 私钥,派生 API key

```go
client, _ := polymarket.NewClobClient("https://clob.polymarket.com", 137, privateKey, nil, nil, "")
creds, _ := client.CreateOrDeriveAPIKey(nil)
client.SetAPICreds(creds)
```

### Level 2 — 下 V2 限价单

```go
client, _ := polymarket.NewClobClient(host, 137, privateKey, creds, nil, "")

resp, err := client.CreateAndPostOrderV2(
    &polymarket.OrderArgsV2{
        TokenID: "713210...",
        Price:   0.4,
        Size:    100,
        Side:    polymarket.BUY,
    },
    nil,                        // PartialCreateOrderOptions
    polymarket.OrderTypeGTC,
    false, false,               // postOnly, deferExec
)
```

### POLY_1271(Deposit Wallet,智能合约钱包)

```go
sig := polymarket.SigTypePoly1271
client, _ := polymarket.NewClobClient(host, 137, eoaPrivateKey, creds, &sig, depositWalletAddress)
```

### 订单类型

| Type | 行为 |
|---|---|
| `GTC` | 一直挂着直到撤单 — 限价默认 |
| `GTD` | 到 `Expiration` 过期 |
| `FOK` | 全成或撤 — 市价默认 |
| `FAK` | 部分成交,剩余撤 |

`PostOnly` 只对 `GTC` / `GTD` 生效。通过 `CreateAndPostOrderV2(..., postOnly=true, ...)` 传入。

### 市价单(V2)

```go
resp, _ := client.CreateAndPostMarketOrderV2(
    &polymarket.MarketOrderArgsV2{
        TokenID:   "713210...",
        Amount:    100,   // BUY 是 USDC 金额,SELL 是份额
        Side:      polymarket.BUY,
        OrderType: polymarket.OrderTypeFOK,
    },
    nil, polymarket.OrderTypeFOK, false,
)
```

### 余额自动保护(V2 限定)

BUY 单传 `UserUsdcBalance`,SDK 根据市场费率公式自动缩 size,确保总成本
(maker + 平台费 + builder fee)不超过余额。见 `examples/place_order_v2/`。

## 示例

| 目录 | 用途 |
|---|---|
| `examples/place_order_v2/` | V2 限价 / 市价下单 + 撤单 |
| `examples/deposit_wallet_buy/` | Polymarket Deposit Wallet (POLY_1271) 下单 |
| `examples/builder_api_key/` | 创建 / 列表 / 撤销 builder API key,查 builder fee rate |
| `examples/check_balance/` | 查 pUSD + CTF 余额 |
| `examples/get_orders/` | 列出开放订单 |
| `examples/market/` | 市场数据只读 |
| `examples/split/` `merge/` `redeem/` | 链上 CTF 操作,需自付 gas |
| `examples/gasless_*/` | 通过 Polymarket relay 提交(免 gas) |

## V2-only

本 SDK **只支持 V2**。预 V2 公共接口(`CreateOrder`、`CreateMarketOrder`、
`CreateAndPostOrder`、`PostOrder`、`PostOrders`、`PostOrderWithOptions`,以及
`OrderArgs` / `MarketOrderArgs` / `PostOrdersArgs` / `PostOrderResult` /
`PostOrdersResult` 类型)已被移除。请改用 V2 等价方法:

- `CreateOrderV2` / `CreateMarketOrderV2`
- `CreateAndPostOrderV2` / `CreateAndPostMarketOrderV2`
- `PostOrderV2` / `PostOrdersV2`
- 入参:`OrderArgsV2` / `MarketOrderArgsV2`

底层 V1 订单构建器仍然保留 —— **仅仅因为 py-clob-client-v2 的 RFQ 子模块还在
构造 V1 订单**;`CreateOrderForRFQ` 在内部走 V1 路径。普通用户不会触及任何 V1 类型。

Web3 授权:`SetAllApprovals()` 现在只授权 V2 Exchange 合约(加上共享的
ConditionalTokens / NegRiskAdapter)。如果你需要 V1 Exchange 授权(基本只在
处理历史 V1 condition 时才用),直接 `SetCollateralApproval(v1ExchangeAddr)`。

Collateral:V2 用 **pUSD**(`0xC011a7E12a19f7B1f670d46F03B03f3342E82DFB`)。
钱包里的 V1 老 condition 仍是用 USDC.e 计价 —— redeem 它们时需要把
`RedeemRequest.Collateral` 设成 `web3.CollateralUSDCe`,或者用
`client.ResolveCollateralFromAsset(...)` 自动检测(见
`examples_local/gasless_batch_redeem/`)。

## 环境变量

示例程序使用的环境变量:

- `PRIVATE_KEY`(必填)
- `CHAIN_ID`(默认 137)
- `SIGNATURE_TYPE`(0=EOA,1=Proxy,2=Safe,3=POLY_1271 Deposit Wallet)
- `FUNDER`(proxy / deposit wallet 必填)
- `CLOB_HOST`(默认 `https://clob.polymarket.com`)
- `CLOB_API_KEY` / `CLOB_SECRET` / `CLOB_PASSPHRASE`(L2)
- `TOKEN_ID`(特定市场)
- `BUILDER_CODE`(V2,可选)
- `USER_USDC_BALANCE`(V2 BUY,可选自动缩量)

## 参考

- [py-clob-client-v2](https://github.com/Polymarket/py-clob-client-v2) — 官方 Python V2 客户端(本 SDK 对齐目标)
- [py-clob-client](https://github.com/Polymarket/py-clob-client) — V1(2026-05 已 archive)
- [Polymarket CLOB 文档](https://docs.polymarket.com/developers/CLOB)

## 许可证

本项目采用 MIT 许可证。

## V2 分支的破坏性变更

如果你之前用过 V2 builder API key 方法,返回类型已经改变:

| 之前 | 现在 |
|---|---|
| `client.CreateBuilderAPIKeyCreds(builderCode)` → `*BuilderApiKeyResponse{APIKey, APISecret, APIPassphrase, BuilderCode}` | `client.CreateBuilderAPIKeyCreds()` → `interface{}`(原始响应,对齐 py-clob-client-v2)。Typed 用法请改用 `client.CreateBuilderAPIKey()` → `*BuilderApiKey{Key, Secret, Passphrase}`。 |

原因:旧字段名(`api_key`/`api_secret`/`api_passphrase`)与 V2 服务端实际字段(`key`/`secret`/`passphrase`)完全对不上,旧 typed 结果在实践中只会拿到空字段。新结构和 helper `polymarket.ParseBuilderApiKey(raw)` 同时兼容两种命名,降低踩坑概率。

## 贡献

欢迎贡献!请随时提交 Pull Request。

