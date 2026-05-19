package order_builder

import (
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// 这些 golden 值来自 py-clob-client-v2 (commit b0a97fa)。
// 复现方法:python3 ref/scripts/gen_v2_goldens.py
//
//   私钥:0x11 重复 32 次
//   chainID: 137
//   contract: 0xE111180000d2663C0091e4f400237545B87B996B (V2 Polygon)
//   salt: 12345 (固定)
const (
	testPK            = "1111111111111111111111111111111111111111111111111111111111111111"
	testChainID       = 137
	testContractV2    = "0xE111180000d2663C0091e4f400237545B87B996B"
	testExpectedAddr  = "0x19E7E376E7C213B7E7e7e46cc70A5dD086DAff2A"
	testExpectedAppDS = "0x3264e159346253e26a64e00b69032db0e7d32f94628de3e6eecb50304d7af3d2"
)

func loadTestKey(t *testing.T) interface{} {
	t.Helper()
	pk, err := crypto.HexToECDSA(testPK)
	if err != nil {
		t.Fatalf("load key: %v", err)
	}
	return pk
}

func newTestBuilder(t *testing.T) *ExchangeOrderBuilderV2 {
	t.Helper()
	pk, err := crypto.HexToECDSA(testPK)
	if err != nil {
		t.Fatalf("load key: %v", err)
	}
	b, err := NewExchangeOrderBuilderV2(testContractV2, testChainID, pk, func() *big.Int { return big.NewInt(12345) })
	if err != nil {
		t.Fatalf("new builder: %v", err)
	}
	return b
}

// TestTypeHashesMatchPython — 预计算 type hash 必须与 Python 完全一致。
func TestTypeHashesMatchPython(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"order_type", "0x" + hex.EncodeToString(v2OrderTypeHash[:]),
			"0xbb86318a2138f5fa8ae32fbe8e659f8fcf13cc6ae4014a707893055433818589"},
		{"solady_type", "0x" + hex.EncodeToString(v2SoladyTypeHash[:]),
			"0x6ba028565cb324c2aa02bb714b9816d0bddd557a2f33bb36cf13272a4256bd42"},
		{"domain_type", "0x" + hex.EncodeToString(v2DomainTypeHash[:]),
			"0x8b73c3c69bb8fe3d512ecc4cf759cc79239f7b179b0ffacaa9a75d522b39400f"},
		{"ctf_name", "0x" + hex.EncodeToString(v2CTFExchangeNameHash[:]),
			"0xf30041e9aac4c4d3a1481d2941dfb0a844a72040e9bbc79a810d1ec5b5d6c7af"},
		{"ctf_version", "0x" + hex.EncodeToString(v2CTFExchangeVersionHash[:]),
			"0xad7c5bef027816a800da1736444fb58a807ef4c9603b7848673f7e3a68eb14a5"},
		{"deposit_name", "0x" + hex.EncodeToString(v2DepositWalletNameHash[:]),
			"0xd682b529a17cda19aa275f3a050608f9e9401fadd1b0d233d81519972295828b"},
		{"deposit_version", "0x" + hex.EncodeToString(v2DepositWalletVersionHash[:]),
			"0xc89efdaa54c0f20c7adf612882df0950f5a951637e0307cdcb4c672f298b8bc6"},
	}
	for _, c := range cases {
		if !strings.EqualFold(c.got, c.want) {
			t.Errorf("%s hash mismatch:\n got  %s\n want %s", c.name, c.got, c.want)
		}
	}
}

// TestAppDomainSeparatorMatchesPython — V2 Exchange 域分隔符必须与 Python 一致。
func TestAppDomainSeparatorMatchesPython(t *testing.T) {
	b := newTestBuilder(t)
	ds := b.AppDomainSeparator()
	got := "0x" + hex.EncodeToString(ds[:])
	if !strings.EqualFold(got, testExpectedAppDS) {
		t.Errorf("app_domain_separator mismatch:\n got  %s\n want %s", got, testExpectedAppDS)
	}
}

// TestSignerAddressFromPrivateKey — sanity check:由测试私钥推出的地址匹配 Python。
func TestSignerAddressFromPrivateKey(t *testing.T) {
	b := newTestBuilder(t)
	got := b.SignerAddress().Hex()
	if !strings.EqualFold(got, testExpectedAddr) {
		t.Errorf("signer addr mismatch:\n got  %s\n want %s", got, testExpectedAddr)
	}
}

// TestV2OrderStructHashesAndSignaturesEOA — 复现 py-clob-client-v2 的 EOA 路径。
func TestV2OrderStructHashesAndSignaturesEOA(t *testing.T) {
	b := newTestBuilder(t)

	cases := []struct {
		name             string
		data             *OrderDataV2
		wantStructHash   string
		wantTypedDigest  string
		wantSignature    string
	}{
		{
			name: "eoa_buy",
			data: &OrderDataV2{
				Maker:         testExpectedAddr,
				Signer:        testExpectedAddr,
				TokenID:       "71321045679252212594626385532706912750332728571942532289631379312455583992563",
				MakerAmount:   "40000000",
				TakerAmount:   "100000000",
				Side:          V2SideBuy,
				SignatureType: V2SigTypeEOA,
				Timestamp:     "1747400000000",
				Metadata:      "",
				Builder:       "",
				Expiration:    "0",
			},
			wantStructHash:  "0x9ff0f24d7c2b9ce8e0d00d74c068b79585edf0445ba4e6ceba0d5d50ad04945f",
			wantTypedDigest: "0x697db959a34d7f3ecbfe11c3b94e797af714f7f9b09fd86d8e907a849c2944b9",
			wantSignature:   "0x3aaf5233d19a802182fd1e7fd1a7600be6ba8914217ba6ea2fe657ccd8336cdd7d456d823e879da133b55e5e90df03a03b4990e9e51d2d9b90b23a71e1d6a5931b",
		},
		{
			name: "eoa_sell_builder",
			data: &OrderDataV2{
				Maker:         testExpectedAddr,
				Signer:        testExpectedAddr,
				TokenID:       "123456789",
				MakerAmount:   "50000000",
				TakerAmount:   "25000000",
				Side:          V2SideSell,
				SignatureType: V2SigTypeEOA,
				Timestamp:     "1747400111222",
				Metadata:      "0xaabbccddeeff00112233445566778899aabbccddeeff00112233445566778899",
				Builder:       "0x1111111111111111111111111111111111111111111111111111111111111111",
				Expiration:    "2000000000",
			},
			wantStructHash:  "0xaada374ab308bc894ea853fb4b001488197ca1b2456c35fd367edba288dc3c97",
			wantTypedDigest: "0xe458d1913ca68e2298e1c21e7a665651fa3d1a086bb90dbfaca3cb3e68095d88",
			wantSignature:   "0xc2daa35b2f48ac1257878ccef7df79400990058c022a7888516a526508a433a00d172c6475a967693c52dc779b937d03eea61d3e55a4a46a6df1d7610409fa4c1b",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			order, err := b.BuildOrder(c.data)
			if err != nil {
				t.Fatalf("BuildOrder: %v", err)
			}
			ch, err := b.ContentsHash(order)
			if err != nil {
				t.Fatalf("ContentsHash: %v", err)
			}
			gotCH := "0x" + hex.EncodeToString(ch[:])
			if !strings.EqualFold(gotCH, c.wantStructHash) {
				t.Errorf("struct hash mismatch:\n got  %s\n want %s", gotCH, c.wantStructHash)
			}

			digest, err := b.OrderTypedDataHash(order)
			if err != nil {
				t.Fatalf("OrderTypedDataHash: %v", err)
			}
			gotDigest := "0x" + hex.EncodeToString(digest[:])
			if !strings.EqualFold(gotDigest, c.wantTypedDigest) {
				t.Errorf("typed data digest mismatch:\n got  %s\n want %s", gotDigest, c.wantTypedDigest)
			}

			sig, err := b.SignOrder(order)
			if err != nil {
				t.Fatalf("SignOrder: %v", err)
			}
			if !strings.EqualFold(sig, c.wantSignature) {
				t.Errorf("signature mismatch:\n got  %s\n want %s", sig, c.wantSignature)
			}
		})
	}
}

// TestV2Poly1271Signature — 复现 py-clob-client-v2 的 POLY_1271 包装签名。
func TestV2Poly1271Signature(t *testing.T) {
	b := newTestBuilder(t)
	depositWallet := "0x9999888877776666555544443333222211110000"
	data := &OrderDataV2{
		Maker:         depositWallet,
		Signer:        depositWallet,
		TokenID:       "999",
		MakerAmount:   "10000000",
		TakerAmount:   "50000000",
		Side:          V2SideBuy,
		SignatureType: V2SigTypePoly1271,
		Timestamp:     "1747400222333",
		Metadata:      "",
		Builder:       "",
		Expiration:    "0",
	}
	wantStructHash := "0x2f7bc16459cc4af2a8e59194204cb841820290042b991287fc05028e7e8d7e48"
	wantSignature := "0xaffb76d5cee936f32f50f72381a8aa9864f41369813ba3d6ef0654bb956d8e384c7674bae3442a262cfa6c7c35b13b3ce5181e7e69bc45f4e908afc46a40bb781c3264e159346253e26a64e00b69032db0e7d32f94628de3e6eecb50304d7af3d22f7bc16459cc4af2a8e59194204cb841820290042b991287fc05028e7e8d7e484f726465722875696e743235362073616c742c61646472657373206d616b65722c61646472657373207369676e65722c75696e7432353620746f6b656e49642c75696e74323536206d616b6572416d6f756e742c75696e743235362074616b6572416d6f756e742c75696e743820736964652c75696e7438207369676e6174757265547970652c75696e743235362074696d657374616d702c62797465733332206d657461646174612c62797465733332206275696c6465722900ba"

	order, err := b.BuildOrder(data)
	if err != nil {
		t.Fatalf("BuildOrder: %v", err)
	}
	ch, err := b.ContentsHash(order)
	if err != nil {
		t.Fatalf("ContentsHash: %v", err)
	}
	gotCH := "0x" + hex.EncodeToString(ch[:])
	if !strings.EqualFold(gotCH, wantStructHash) {
		t.Fatalf("struct hash mismatch:\n got  %s\n want %s", gotCH, wantStructHash)
	}

	sig, err := b.SignOrder(order)
	if err != nil {
		t.Fatalf("SignOrder: %v", err)
	}
	if !strings.EqualFold(sig, wantSignature) {
		t.Errorf("poly1271 signature mismatch:\n got  %s\n want %s", sig, wantSignature)
	}
}

// TestSignerMismatchEOA — 非 POLY_1271 路径下,signer 与签名者私钥不匹配应报错。
func TestSignerMismatchEOA(t *testing.T) {
	b := newTestBuilder(t)
	_, err := b.BuildOrder(&OrderDataV2{
		Maker:         "0x0000000000000000000000000000000000000001",
		Signer:        "0x0000000000000000000000000000000000000001", // 与私钥推出的地址不符
		TokenID:       "1",
		MakerAmount:   "1",
		TakerAmount:   "1",
		Side:          V2SideBuy,
		SignatureType: V2SigTypeEOA,
		Timestamp:     "1",
	})
	if err == nil {
		t.Fatal("expected signer mismatch error, got nil")
	}
}

// TestPoly1271AllowsMismatchedSigner — POLY_1271 允许 signer 是 Deposit Wallet。
func TestPoly1271AllowsMismatchedSigner(t *testing.T) {
	b := newTestBuilder(t)
	_, err := b.BuildOrder(&OrderDataV2{
		Maker:         "0xaaaa000000000000000000000000000000000001",
		Signer:        "0xaaaa000000000000000000000000000000000001",
		TokenID:       "1",
		MakerAmount:   "1",
		TakerAmount:   "1",
		Side:          V2SideBuy,
		SignatureType: V2SigTypePoly1271,
		Timestamp:     "1",
	})
	if err != nil {
		t.Fatalf("POLY_1271 should not require signer to match: %v", err)
	}
}

// TestHexTo32BytesPadsLeft — 验证 hex → bytes32 的左 pad 行为(含奇数长度自动补 0)。
func TestHexTo32BytesPadsLeft(t *testing.T) {
	cases := []struct {
		in   string
		want [32]byte
	}{
		{"", [32]byte{}},
		{"0x", [32]byte{}},
		{"0x01", func() [32]byte { var b [32]byte; b[31] = 1; return b }()},
		{"0xff", func() [32]byte { var b [32]byte; b[31] = 0xff; return b }()},
		// 奇数长度:0xa → "0a" → bytes32 末尾 0x0a
		{"0xa", func() [32]byte { var b [32]byte; b[31] = 0x0a; return b }()},
		// 奇数长度多字节:0x1a2 → "01a2" → bytes32 末尾 0x01 0xa2
		{"0x1a2", func() [32]byte { var b [32]byte; b[30] = 0x01; b[31] = 0xa2; return b }()},
		// 不带 0x 前缀
		{"abcd", func() [32]byte { var b [32]byte; b[30] = 0xab; b[31] = 0xcd; return b }()},
	}
	for _, c := range cases {
		got, err := hexTo32Bytes(c.in)
		if err != nil {
			t.Fatalf("input %q: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("input %q: got %x, want %x", c.in, got, c.want)
		}
	}
}

// TestHexTo32BytesRejectsBadHex — 非 hex 字符应报错。
func TestHexTo32BytesRejectsBadHex(t *testing.T) {
	if _, err := hexTo32Bytes("0xZZZZ"); err == nil {
		t.Error("expected error for non-hex chars")
	}
}

// TestHexTo32BytesRejectsOversize — 超过 32 字节应报错。
func TestHexTo32BytesRejectsOversize(t *testing.T) {
	_, err := hexTo32Bytes("0x" + strings.Repeat("aa", 33))
	if err == nil {
		t.Fatal("expected error for >32 byte input")
	}
}

// TestDefaultSaltGeneratorRange — salt 应落在 [0, 2^32) 区间。
func TestDefaultSaltGeneratorRange(t *testing.T) {
	max := new(big.Int).Lsh(big.NewInt(1), 32)
	for i := 0; i < 32; i++ {
		s := defaultSaltGenerator()
		if s.Sign() < 0 || s.Cmp(max) >= 0 {
			t.Errorf("salt out of range: %s", s.String())
		}
	}
}

// TestToJSONPayloadShape — 序列化后的 JSON 结构和字段名与 py-clob-client-v2 对齐。
func TestToJSONPayloadShape(t *testing.T) {
	b := newTestBuilder(t)
	order, err := b.BuildSignedOrder(&OrderDataV2{
		Maker:         testExpectedAddr,
		Signer:        testExpectedAddr,
		TokenID:       "1",
		MakerAmount:   "1000000",
		TakerAmount:   "2000000",
		Side:          V2SideBuy,
		SignatureType: V2SigTypeEOA,
		Timestamp:     "1700000000000",
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	payload := order.ToJSONPayload("owner-key", "GTC", false, false)
	orderField, ok := payload["order"].(map[string]interface{})
	if !ok {
		t.Fatal("order field missing")
	}
	for _, key := range []string{"salt", "maker", "signer", "tokenId", "makerAmount", "takerAmount",
		"side", "expiration", "signatureType", "timestamp", "metadata", "builder", "signature"} {
		if _, ok := orderField[key]; !ok {
			t.Errorf("order field missing key %q", key)
		}
	}
	for _, key := range []string{"order", "owner", "orderType", "postOnly", "deferExec"} {
		if _, ok := payload[key]; !ok {
			t.Errorf("payload missing top-level key %q", key)
		}
	}
	if side := orderField["side"]; side != "BUY" {
		t.Errorf("side should be string 'BUY', got %v", side)
	}
}

// TestToJSONPayloadSaltSurvivesUint256Range — 测试外部 saltGenerator 返回
// 超过 int64 范围的 uint256 时,salt 在 JSON payload 里依然是完整的十进制
// 数字(无截断、无引号)。修复 Int64() 截断 bug。
func TestToJSONPayloadSaltSurvivesUint256Range(t *testing.T) {
	pk, _ := crypto.HexToECDSA(testPK)
	bigSalt, _ := new(big.Int).SetString("123456789012345678901234567890", 10) // > int64
	b, err := NewExchangeOrderBuilderV2(testContractV2, testChainID, pk, func() *big.Int { return bigSalt })
	if err != nil {
		t.Fatalf("new builder: %v", err)
	}
	order, err := b.BuildSignedOrder(&OrderDataV2{
		Maker:         testExpectedAddr,
		Signer:        testExpectedAddr,
		TokenID:       "1",
		MakerAmount:   "1000000",
		TakerAmount:   "2000000",
		Side:          V2SideBuy,
		SignatureType: V2SigTypeEOA,
		Timestamp:     "1700000000000",
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	payload := order.ToJSONPayload("k", "GTC", false, false)
	js, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// 应该包含完整的 salt 数字(无截断、无引号)
	want := `"salt":123456789012345678901234567890`
	if !strings.Contains(string(js), want) {
		t.Errorf("expected %q in JSON, got %s", want, string(js))
	}
}

// TestBuildOrderRejectsInvalidMakerSigner — 非法 maker / signer 必须 error,
// 而不是被 common.HexToAddress 静默截成 0x000...000。
func TestBuildOrderRejectsInvalidMakerSigner(t *testing.T) {
	b := newTestBuilder(t)

	// maker 非法字符串
	_, err := b.BuildOrder(&OrderDataV2{
		Maker:         "not-an-address",
		Signer:        testExpectedAddr,
		TokenID:       "1",
		MakerAmount:   "1",
		TakerAmount:   "1",
		Side:          V2SideBuy,
		SignatureType: V2SigTypeEOA,
	})
	if err == nil || !strings.Contains(err.Error(), "invalid maker") {
		t.Errorf("expected invalid maker error, got %v", err)
	}

	// signer 非法字符串
	_, err = b.BuildOrder(&OrderDataV2{
		Maker:         testExpectedAddr,
		Signer:        "0xZZZZ",
		TokenID:       "1",
		MakerAmount:   "1",
		TakerAmount:   "1",
		Side:          V2SideBuy,
		SignatureType: V2SigTypeEOA,
	})
	if err == nil || !strings.Contains(err.Error(), "invalid signer") {
		t.Errorf("expected invalid signer error, got %v", err)
	}

	// 短地址(只有 0x)也应该拒绝
	_, err = b.BuildOrder(&OrderDataV2{
		Maker:         "0x",
		Signer:        testExpectedAddr,
		TokenID:       "1",
		MakerAmount:   "1",
		TakerAmount:   "1",
		Side:          V2SideBuy,
		SignatureType: V2SigTypeEOA,
	})
	if err == nil {
		t.Error("expected error for short address '0x'")
	}
}

// TestV2BuilderRejectsBadAddress — 非法合约地址应报错。
func TestV2BuilderRejectsBadAddress(t *testing.T) {
	pk, _ := crypto.HexToECDSA(testPK)
	_, err := NewExchangeOrderBuilderV2("not-an-address", testChainID, pk, nil)
	if err == nil {
		t.Fatal("expected error for invalid contract address")
	}
}

// 确保 common 包仍在使用(避免 unused import)
var _ = common.HexToAddress
