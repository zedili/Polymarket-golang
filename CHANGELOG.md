# Changelog

All notable changes to the Polymarket Go SDK.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

(Nothing planned.)

---

## [0.4.0] — 2026-05-20

Adds `context.Context` and typed responses on hot-path methods. **Not breaking** —
all existing method signatures preserved; new variants live alongside.

### Added — `*Ctx` variants

Caller-cancellable HTTP for the most-used public methods. Original methods
delegate via `context.Background()`.

- `CancelCtx`, `CancelOrdersCtx`, `CancelAllCtx`, `CancelMarketOrdersCtx`
- `PostOrderV2Ctx`, `CreateAndPostOrderV2Ctx`, `CreateAndPostMarketOrderV2Ctx`
- `GetBalanceAllowanceCtx`

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
result, err := client.CreateAndPostOrderV2Ctx(ctx, args, nil, polymarket.OrderTypeGTC, false, false)
```

### Added — Typed responses

Strongly-typed structs (`OpenOrder`, `Trade`, `MakerOrderRef`, `CancelResult`) +
`*Typed` method variants that decode the existing `interface{}` returns. Save your
callers from manual `map[string]interface{}` gymnastics.

- `GetOrdersTyped` / `GetOrdersTypedCtx` → `[]OpenOrder`
- `GetTradesTyped` / `GetTradesTypedCtx` → `[]Trade`
- `GetOrderTyped` → `*OpenOrder`
- `CancelTyped` / `CancelOrdersTyped` / `CancelAllTyped` → `*CancelResult`

```go
orders, _ := client.GetOrdersTyped(nil, "")
for _, o := range orders {
    fmt.Printf("%s %s @ %s\n", o.Side, o.OriginalSize, o.Price) // IDE autocomplete!
}
```

Implementation note: `*Typed` methods walk the original `interface{}` path then
JSON-roundtrip into the typed struct (~10 µs overhead). Original `interface{}` API
remains for callers who decode manually or have unusual response shapes.

---

## [0.3.0] — 2026-05-19

**Major release. Full migration to Polymarket CLOB V2.** The upstream py-clob-client
was archived 2026-05; this SDK now mirrors `py-clob-client-v2` semantics.

### Breaking changes

- **V1 CLOB API surface removed**. `ClobClient`'s V1 order creation / posting methods
  no longer exist. Use `CreateOrderV2`, `PostOrderV2`, `CreateAndPostOrderV2`,
  `CreateMarketOrderV2`, `CreateAndPostMarketOrderV2`.
- **Default collateral switched to pUSD** (`0xC011a7E12a19f7B1f670d46F03B03f3342E82DFB`).
  `client.USDCAddress` now points at pUSD; V1 USDC.e address kept as
  `web3.CollateralUSDCe`.
- **`web3.SetAllApprovals`** simplified to V2-only (no longer approves V1 exchanges).
- **`OrderArgs` → `OrderArgsV2`**: drops `taker`, `nonce`, `feeRateBps`;
  adds `timestamp`, `metadata`, `builder_code`.

### Added — Trading core

- **V2 EIP-712 order builder** (`polymarket/order_builder/exchange_order_builder_v2.go`).
  - V2 domain version "2"; new type strings.
  - Correct signer mapping per `SignatureTypeV2`:
    - `0` EOA / `1` PolyProxy / `2` PolyGnosisSafe → `signer = EOA`, `maker = funder`
    - `3` POLY_1271 (Deposit Wallet) → `signer = funder`, with Solady `TypedDataSign` wrapper
- **Builder API key lifecycle**: `CreateBuilderAPIKey`, `GetBuilderAPIKeysList`,
  `RevokeBuilderAPIKeyCreds`, `GetBuilderFeeRate`. Builder code goes into each order
  via `OrderArgsV2.BuilderCode` / `SetBuilderConfig`.
- **Fee handling**: `AdjustBuyAmountForFees`, `ValidateFeeSlippage`,
  `adjust_buy_amount_for_balance` parity with Python V2.
- **Version negotiation**: server `/version` cached; `IsOrderVersionMismatch` triggers
  one safe retry (with 100ms backoff) per `CreateAndPostOrderV2` call.

### Added — On-chain (gasless)

- **`PolymarketGaslessWeb3Client.WrapUSDCeToPUSD(amount, recipient)`** —
  batches `USDC.e.approve(onramp)` + `CollateralOnramp.wrap(...)` in one relay call.
  Polygon CollateralOnramp address: `0x93070a847efEf7F70739046A929D47a521F5B8ee`.
- **Auto-detect collateral** for redeem (`ResolveCollateralFromAsset[Batch]`):
  V1 conditions return USDC.e, V2 conditions return pUSD; the SDK queries on-chain to know.
- **`RedeemRequest.Collateral`** field — must match the condition's actual collateral,
  or `redeemPositions` computes the wrong positionId and the relayer rejects.
- **`SetMinConfirmations(n)`** on the gasless client — wait for N block confirmations
  before returning a receipt (default 1; recommend 3 for serious PnL tracking).

### Added — Endpoints / Data

- **Rewards / Rebates** (`polymarket/client_rewards.go`): `GetEarningsForUserForDay`,
  `GetTotalEarningsForUserForDay`, `GetLiquidityRewardPercentages`,
  `GetRewardsMarketsCurrent`, `GetRewardsMarketsForUser`, `GetRewardsMarketsForCondition`,
  `GetCurrentMakerRebate`.
- **Bridge** (`polymarket/bridge_client.go`): `GetSupportedAssets`, `GetQuote`,
  `CreateDepositAddresses`, `CreateWithdrawAddresses`, `GetStatus`. Independent
  `https://bridge.polymarket.com` base URL, no auth needed.
- **Data API** (`polymarket/data_api.go`): positions / value / holdings / activity /
  holders queries.
- **Gamma API** (`polymarket/gamma_api.go`): markets / events / series / tags lookups.
- **Misc**: `GetOpenInterest`, `GetLiveVolume`, `GetTotalMarketsTraded`.

### Added — Real-time

- **WebSocket client** (`polymarket/ws_client.go`):
  - `/ws/market` (no auth, multi-asset book + price_change + last_trade + tick_size_change)
  - `/ws/user` (L2 auth, trade + order events) — **must use EOA-derived key, NOT
    builder API key** (server closes builder key sessions with code 1006).
  - Strongly-typed dispatcher with `WSHandler` callbacks.
  - `SetReadDeadline` (3.5× ping) + `SetPongHandler` + `SetReadLimit` (1 MiB) prevent
    silent freeze on half-open TCP.
  - `json.Unmarshal` errors surface via `HandleUnknown("<event_type>:decode_error", raw)`
    so schema drift doesn't silently drop user `trade`/`order` events.
  - Sentinel errors for reconnect logic: `ErrWSServerClose`, `ErrWSAuthRejected`,
    `ErrWSReadTimeout`.
  - Configurable ping interval + read limit; ping has ±10% jitter to avoid lockstep
    when running many instances.

### Fixed — Bugs that affected real users

- **`web3.WaitForReceipt(txHash)` was completely broken** — it discarded `txHash` and
  passed an empty `Transaction{}` to `bind.WaitMined`. Now polls `TransactionReceipt`
  by hash. New `WaitForReceiptCtx(ctx, hash, timeout)` variant.
- **WebSocket silent freeze** on half-open TCP — no `SetReadDeadline` was ever set.
  Fixed (see WS section above).
- **`waitForReceipt` early return** could be re-orged on Polygon — fixed by
  opt-in `SetMinConfirmations`.

### Fixed — Polish

- **Library no longer prints to stdout.** All `fmt.Printf` in `web3/gasless_client.go`
  replaced with pluggable `Logger` interface (`web3.SetLogger`, defaults to stderr).
- **HTTP response size cap** (`io.LimitedReader`, 16 MiB) — prevents OOM on bad upstream.
- **Concurrent nonce race** in `GetTransactionOpts` — added per-client `cachedNonce`
  + `InvalidateNonce()` for resync.
- **`collateral_detect.Batch` spawned all goroutines at once** on large queries —
  semaphore now acquired before `go func`, properly bounding concurrency.
- **`getOrCreateV2Builder` double-check** — write lock now re-checks cache to avoid
  duplicate `NewExchangeOrderBuilderV2` calls (which do an EIP-712 domain hash).
- **`gorilla/websocket` upgraded** v1.4.2 → v1.5.3 (CVE patches).
- **gasless `http.Client` zero timeout + no shared transport** — now 30s timeout +
  pooled transport. Prevents hung relay blocking caller.
- **HMAC pre-decode + buffer pool** in HTTP body marshaling; HTTP/2 + connection
  pooling on main `httpClient`.
- **`retryOnVersionUpdate` no backoff** between retries — added 100ms sleep.
- **Builder API key field naming** aligned with Python V2 (`key/secret/passphrase`
  not `api_key/api_secret/api_passphrase`); `ParseBuilderApiKey` doesn't leak secrets
  on parse error.
- **`builderConfig` data race** fixed via `defaultBuilderCode()` helper with RLock.
- **`GetBuilderTrades` query** now uses `url.Values` (not string concat).
- **`CreateMarketOrderV2`** no longer mutates caller's `args.Price`/`args.BuilderCode`.
- **V2 `ToJSONPayload` salt** uses `*big.Int` (no more `int64` overflow risk).
- **Address validation** added in `NewClobClient` and `BuildOrder`
  (`common.IsHexAddress`).
- **Amoy NegRiskExchange V1 address** corrected (was using adapter address).

### Tests

- Full unit test suite green: `go test ./...` (~6s).
- Race detector clean: `go test -race ./...` (~9s).
- HMAC + V2 builder benchmarks under `polymarket/hmac_bench_test.go`,
  `polymarket/order_builder/amounts_v2_test.go`.
- Real-world e2e validated against Polygon mainnet with a small-balance test wallet:
  - V2 limit GTC order (placed + cancelled)
  - V2 market FOK BUY (Reya YES 4.54 share)
  - V2 market FOK BUY (BTC 5min NO 2 share)
  - V2 market FOK SELL (Reya YES 4.54 share → 0.82 pUSD)
  - Gasless `redeemPositions` on USDC.e condition (2 USDC.e in)
  - Gasless `wrap` USDC.e → pUSD (2 USDC.e → 2 pUSD)
  - WS `/ws/market` + `/ws/user` over 15s (book + price_change + trade events).

---

## [0.2.12] and earlier

V1-era releases. No structured changelog kept.

[Unreleased]: https://github.com/0xNetuser/Polymarket-golang/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/0xNetuser/Polymarket-golang/compare/v0.2.12...v0.3.0
