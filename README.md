# Polymarket Go SDK

[English](README.md) | [中文](README_zh.md)

A comprehensive Go SDK for the Polymarket CLOB. Aligned with both [py-clob-client](https://github.com/Polymarket/py-clob-client) (V1, archived 2026-05) and [py-clob-client-v2](https://github.com/Polymarket/py-clob-client-v2) (V2, current).

Follow at X:  @netu5er

## Features

- ✅ **V2-native** — aligned with `py-clob-client-v2` (V1 was archived 2026-05)
- ✅ **CTF Exchange V2** order signing (EIP-712 EOA + EIP-1271 Solady wrapped for Deposit Wallet)
- ✅ **Automatic version negotiation** — `/version` cache + retry on `order_version_mismatch`
- ✅ **Builder code / Builder API key** for fee attribution
- ✅ **Three Authentication Levels**: L0, L1, L2 (with HMAC body signing aligned to V2)
- ✅ **Order Management**: limit + market, post-only, GTC/GTD/FOK/FAK, batch
- ✅ **RFQ**: request-for-quote flow
- ✅ **WebSocket** — `/ws/market` + `/ws/user` with typed dispatcher, read deadline, reconnect-aware sentinels
- ✅ **Bridge** — cross-chain deposit/withdraw via `bridge.polymarket.com`
- ✅ **Rewards / Rebates** — full earnings + market reward configs
- ✅ **Gasless on-chain ops** — split / merge / redeem / convert / wrap USDC.e→pUSD
- ✅ **Strong typing** + byte-for-byte tested against py-clob-client-v2 golden signatures

## What's new in v0.3.0

Full V2 migration plus four new module groups. Quick taste of each — see
[CHANGELOG.md](CHANGELOG.md) for the full list.

### WebSocket (real-time book + user events)

```go
// market channel (no auth)
mc, _ := polymarket.NewMarketWSClient(
    []string{tokenID}, true, // custom_feature_enabled=true also delivers new_market events
    &polymarket.WSHandler{
        OnBook:        func(e polymarket.WSBookEvent)        { /* full snapshot */ },
        OnPriceChange: func(e polymarket.WSPriceChangeEvent) { /* deltas */ },
        OnLastTradePrice: func(e polymarket.WSLastTradePriceEvent) { /* trades */ },
    },
)
go mc.Run(ctx) // blocks until ctx done / ErrWSServerClose / ErrWSReadTimeout

// user channel (L2 auth — must use EOA-derived key, NOT builder key)
uc, _ := polymarket.NewUserWSClient(derivedCreds, []string{conditionID}, &polymarket.WSHandler{
    OnOrder: func(e polymarket.WSOrderEvent) { /* PLACEMENT/UPDATE/CANCELLATION */ },
    OnTrade: func(e polymarket.WSTradeEvent) { /* MATCHED/MINED/CONFIRMED */ },
})
go uc.Run(ctx)
```

Read deadline (3.5× ping) auto-resets on every frame — no more silent freeze on
half-open TCP. Decoder errors surface via `HandleUnknown("<event_type>:decode_error", raw)`
so schema drift doesn't lose user events.

### Wrap USDC.e → pUSD (CollateralOnramp)

After redeeming a V1 USDC.e condition, your proxy wallet holds USDC.e. Polymarket
UI shows it as "Pending deposit" — the SDK can wrap it gaslessly in one batch:

```go
wc, _ := web3.NewPolymarketGaslessWeb3Client(pk, web3.SignatureTypePolyProxy,
    builderCreds, 137, rpcURL)

// approve(USDC.e → Onramp) + Onramp.wrap(USDC.e, recipient, amount) in one relay tx
receipt, _ := wc.WrapUSDCeToPUSD(2.0, common.HexToAddress(funder))
```

### Bridge (cross-chain deposits)

```go
b := polymarket.NewBridgeClient() // independent host: bridge.polymarket.com

assets, _ := b.GetSupportedAssets()       // 209 tokens across many chains
dep, _    := b.CreateDepositAddresses(walletAddress) // returns EVM/SVM/BTC addresses
quote, _  := b.GetQuote(&polymarket.BridgeQuoteRequest{...})
status, _ := b.GetStatus(dep.Address.EVM) // poll progress
```

### Rewards / Rebates

```go
// public — current reward configs
all, _ := client.GetRewardsMarketsCurrent(nil, "")

// L2 — my earnings today
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

## V2 quick start

```go
client, _ := polymarket.NewClobClient("https://clob.polymarket.com", 137, pk, creds, nil, "")

// Optional: pin a builder code globally
client.SetBuilderConfig(&polymarket.BuilderConfig{
    BuilderCode: "0xabc...0001", // bytes32
})

// V2 limit BUY (timestamp/metadata/builder are auto-filled)
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

For Polymarket Deposit Wallet (smart-contract wallet, EIP-1271 / Solady):

```go
sig := polymarket.SigTypePoly1271
client, _ := polymarket.NewClobClient(host, chainID, eoaPrivateKey, creds, &sig, depositWalletAddress)
```

See `examples/place_order_v2/`, `examples/deposit_wallet_buy/`, `examples/builder_api_key/`.

## Installation

```bash
go get github.com/0xNetuser/Polymarket-golang
```

## Quick Start

### Level 0 — read-only

```go
client, _ := polymarket.NewClobClient("https://clob.polymarket.com", 137, "", nil, nil, "")
book, _ := client.GetOrderBook("token-id")
fmt.Printf("server version: %d\n", client.GetVersion())
```

### Level 1 — private key, derive API key

```go
client, _ := polymarket.NewClobClient("https://clob.polymarket.com", 137, privateKey, nil, nil, "")
creds, _ := client.CreateOrDeriveAPIKey(nil)
client.SetAPICreds(creds)
```

### Level 2 — place a V2 limit order

```go
client, _ := polymarket.NewClobClient(host, 137, privateKey, creds, nil, "")

resp, err := client.CreateAndPostOrderV2(
    &polymarket.OrderArgsV2{
        TokenID: "713210...",
        Price:   0.4,
        Size:    100,
        Side:    polymarket.BUY,
    },
    nil,                       // PartialCreateOrderOptions
    polymarket.OrderTypeGTC,
    false, false,              // postOnly, deferExec
)
```

### POLY_1271 (Deposit Wallet, smart-contract wallet)

```go
sig := polymarket.SigTypePoly1271
client, _ := polymarket.NewClobClient(host, 137, eoaPrivateKey, creds, &sig, depositWalletAddress)
```

### Order types

| Type | Behavior |
|---|---|
| `GTC` | Good Till Cancel — default for limit |
| `GTD` | Good Till Date — needs `Expiration` |
| `FOK` | Fill Or Kill — default for market |
| `FAK` | Fill And Kill — partial fill, cancel rest |

`PostOnly` is only valid with `GTC` / `GTD`. Pass via `CreateAndPostOrderV2(..., postOnly=true, ...)`.

### Market order (V2)

```go
resp, _ := client.CreateAndPostMarketOrderV2(
    &polymarket.MarketOrderArgsV2{
        TokenID:   "713210...",
        Amount:    100,   // USDC for BUY, shares for SELL
        Side:      polymarket.BUY,
        OrderType: polymarket.OrderTypeFOK,
    },
    nil, polymarket.OrderTypeFOK, false,
)
```

### Auto balance protection (V2 only)

Pass `UserUsdcBalance` on a BUY order and the SDK will downsize automatically
based on the market's fee curve so total cost (maker + platform fee + builder fee)
stays under your balance. See `examples/place_order_v2/`.

## Examples

| Dir | What it does |
|---|---|
| `examples/place_order_v2/` | Place / cancel V2 limit + market orders |
| `examples/deposit_wallet_buy/` | Place an order from a Polymarket Deposit Wallet (POLY_1271) |
| `examples/builder_api_key/` | Create / list / revoke builder API key, fetch builder fee rate |
| `examples/check_balance/` | Query pUSD + CTF balances |
| `examples/get_orders/` | List open orders |
| `examples/market/` | Read-only market data |
| `examples/split/` `merge/` `redeem/` | On-chain conditional-token ops, pays gas |
| `examples/gasless_*/` | Same ops via Polymarket relay (no gas) |

### Heartbeat API

The Heartbeat API allows you to automatically cancel all orders if your connection is lost. Once started, you must send a heartbeat every 10 seconds or all orders will be cancelled.

```go
// Start heartbeat with an optional ID
heartbeatID := "my-session-123"
result, err := client.PostHeartbeat(&heartbeatID)

// Or send without ID
result, err := client.PostHeartbeat(nil)
```

**Use Case**: Prevent stale orders when your trading system goes offline unexpectedly.

## Web3 Clients

The SDK includes two Web3 clients for on-chain operations:

### PolymarketWeb3Client (Pay Gas)

```go
import "github.com/0xNetuser/Polymarket-golang/polymarket/web3"

// Create Web3 client (pays gas for transactions)
client, err := web3.NewPolymarketWeb3Client(
    "your-private-key",
    web3.SignatureTypePolyProxy, // 0=EOA, 1=PolyProxy, 2=Safe
    137,                         // Chain ID
    "",                          // RPC URL (empty = default)
)

// Get balances
polBalance, _ := client.GetPOLBalance()
usdcBalance, _ := client.GetUSDCBalance(common.Address{})
tokenBalance, _ := client.GetTokenBalance("token-id", common.Address{})

// Set all necessary approvals
receipts, _ := client.SetAllApprovals()

// Split USDC into positions
receipt, _ := client.SplitPosition(conditionID, 100.0, true) // negRisk=true

// Merge positions back to USDC
receipt, _ := client.MergePosition(conditionID, 100.0, true)

// Transfer USDC
receipt, _ := client.TransferUSDC(recipient, 50.0)

// Transfer conditional tokens
receipt, _ := client.TransferToken("token-id", recipient, 50.0)
```

### PolymarketGaslessWeb3Client (No Gas)

```go
// Create Gasless Web3 client (transactions via relay, no gas required)
// Only supports signature_type=1 (PolyProxy) or signature_type=2 (Safe)
client, err := web3.NewPolymarketGaslessWeb3Client(
    "your-private-key",
    web3.SignatureTypePolyProxy,
    nil,  // Optional: builder credentials
    137,
    "",
)

// Same operations as PolymarketWeb3Client
receipt, _ := client.SplitPosition(conditionID, 100.0, true)
receipt, _ := client.MergePosition(conditionID, 100.0, true)
```

## V2-only

This SDK is **V2-only**. The V1 client surface (`CreateOrder`, `CreateMarketOrder`,
`CreateAndPostOrder`, `PostOrder`, `PostOrders`, `PostOrderWithOptions` plus the
`OrderArgs` / `MarketOrderArgs` / `PostOrdersArgs` / `PostOrderResult` /
`PostOrdersResult` types) has been removed. Use the V2 equivalents:

- `CreateOrderV2` / `CreateMarketOrderV2`
- `CreateAndPostOrderV2` / `CreateAndPostMarketOrderV2`
- `PostOrderV2` / `PostOrdersV2`
- input: `OrderArgsV2` / `MarketOrderArgsV2`

The underlying V1 order builder is still present **only because RFQ on
py-clob-client-v2 still constructs V1 orders**; `CreateOrderForRFQ` uses it
internally. Normal users do not touch any V1 type.

Web3 approvals: `SetAllApprovals()` now only authorizes V2 Exchange contracts
(plus the universal ConditionalTokens / NegRiskAdapter for split/merge). If you
need V1 approval for some historical condition you can still call
`SetCollateralApproval(v1ExchangeAddr)` directly.

Collateral: V2 uses **pUSD** (`0xC011a7E12a19f7B1f670d46F03B03f3342E82DFB`).
Old V1-era conditions on a wallet are still denominated in USDC.e — to redeem
them, set `RedeemRequest.Collateral = web3.CollateralUSDCe`, or use
`client.ResolveCollateralFromAsset(...)` to auto-detect (see
`examples_local/gasless_batch_redeem/`).

## Environment Variables

Used by example programs:

- `PRIVATE_KEY` (required)
- `CHAIN_ID` (default 137)
- `SIGNATURE_TYPE` (0=EOA, 1=Proxy, 2=Safe, 3=POLY_1271 Deposit Wallet)
- `FUNDER` (for proxy / deposit wallets)
- `CLOB_HOST` (default `https://clob.polymarket.com`)
- `CLOB_API_KEY` / `CLOB_SECRET` / `CLOB_PASSPHRASE` (L2)
- `TOKEN_ID` (specific market)
- `BUILDER_CODE` (V2, optional)
- `USER_USDC_BALANCE` (V2 BUY, optional auto-downsize)

## References

- [py-clob-client-v2](https://github.com/Polymarket/py-clob-client-v2) — official Python V2 client (this SDK targets parity)
- [py-clob-client](https://github.com/Polymarket/py-clob-client) — V1 (archived 2026-05)
- [Polymarket CLOB Documentation](https://docs.polymarket.com/developers/CLOB)

## Breaking changes (V2 migration)

The pre-V2 surface has been removed. Migration table:

| Removed | Use instead |
|---|---|
| `ClobClient.CreateOrder` / `CreateMarketOrder` / `CreateAndPostOrder` | `CreateOrderV2` / `CreateMarketOrderV2` / `CreateAndPostOrderV2` |
| `ClobClient.PostOrder` / `PostOrderWithOptions` / `PostOrders` | `PostOrderV2` / `PostOrdersV2` |
| `polymarket.OrderArgs` / `MarketOrderArgs` | `OrderArgsV2` / `MarketOrderArgsV2` |
| `polymarket.PostOrdersArgs` / `PostOrderResult` / `PostOrdersResult` | `PostOrdersArgsV2` / `PostOrderResultV2` / `PostOrdersResultV2` |
| `polymarket.OrderToJSON` / `OrderToJSONWithPostOnly` | `SignedOrderV2.ToJSONPayload(...)` |
| `web3.SetV1Approvals` | gone (V1 Exchange not auto-approved anymore) |
| `web3.SetAllApprovals` (V1+V2) | V2-only now |
| `CreateBuilderAPIKeyCreds(builderCode)` returning typed struct | `CreateBuilderAPIKeyCreds()` returns raw response;  `CreateBuilderAPIKey()` is the typed wrapper returning `*BuilderApiKey{Key, Secret, Passphrase}` |

`SignedOrder` (V1 type alias) is kept because the RFQ sub-module on
py-clob-client-v2 still constructs V1 orders; everyday users don't reference it.

## License

This project is licensed under the MIT License.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
