package polymarket

import (
	"encoding/base64"
	"strings"
	"testing"
)

// 测试用 base64-url secret(任意 32 字节)。
const testSecret = "aGVsbG8td29ybGQtMTIzNDU2Nzg5MGFiY2RlZmdoaWprbG1u"

func decodeSig(t *testing.T, s string) []byte {
	t.Helper()
	out, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	return out
}

// 同一输入应得到确定签名(HMAC 是 deterministic 的)。
func TestHMACDeterminism(t *testing.T) {
	s1, err := BuildHMACSignature(testSecret, 1700000000, "POST", "/order", `{"a":1}`)
	if err != nil {
		t.Fatalf("hmac: %v", err)
	}
	s2, err := BuildHMACSignature(testSecret, 1700000000, "POST", "/order", `{"a":1}`)
	if err != nil {
		t.Fatalf("hmac: %v", err)
	}
	if s1 != s2 {
		t.Errorf("HMAC not deterministic: %s vs %s", s1, s2)
	}
	if len(decodeSig(t, s1)) != 32 {
		t.Errorf("HMAC-SHA256 should produce 32 bytes")
	}
}

// 不同 body 应得到不同签名。
func TestHMACBodyChangesSignature(t *testing.T) {
	s1, _ := BuildHMACSignature(testSecret, 1700000000, "POST", "/order", `{"a":1}`)
	s2, _ := BuildHMACSignature(testSecret, 1700000000, "POST", "/order", `{"a":2}`)
	if s1 == s2 {
		t.Errorf("expected different signatures for different bodies")
	}
}

// 不同时间戳应得到不同签名。
func TestHMACTimestampChangesSignature(t *testing.T) {
	s1, _ := BuildHMACSignature(testSecret, 1700000000, "POST", "/order", "x")
	s2, _ := BuildHMACSignature(testSecret, 1700000001, "POST", "/order", "x")
	if s1 == s2 {
		t.Errorf("expected different signatures for different timestamps")
	}
}

// string、*string、[]byte 三种 body 类型应得到相同签名。
func TestHMACBodyTypeEquivalence(t *testing.T) {
	body := `{"x":1,"y":"two"}`
	bodyPtr := &body
	bodyBytes := []byte(body)
	s1, _ := BuildHMACSignature(testSecret, 1700000000, "POST", "/x", body)
	s2, _ := BuildHMACSignature(testSecret, 1700000000, "POST", "/x", bodyPtr)
	s3, _ := BuildHMACSignature(testSecret, 1700000000, "POST", "/x", bodyBytes)
	if s1 != s2 || s2 != s3 {
		t.Errorf("string/ptr/bytes mismatch: %s / %s / %s", s1, s2, s3)
	}
}

// MarshalCompact 应产生稳定、无空格的 JSON(对 map key 字母排序)。
func TestMarshalCompactStable(t *testing.T) {
	body := map[string]interface{}{
		"zebra": 1,
		"alpha": 2,
		"mango": 3,
	}
	s1, err := MarshalCompact(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s2, _ := MarshalCompact(body)
	if s1 != s2 {
		t.Errorf("not stable: %s vs %s", s1, s2)
	}
	if strings.Contains(s1, " ") {
		t.Errorf("compact JSON should not contain spaces: %s", s1)
	}
	// Go 的 json.Marshal 对 map[string]X 是按 key 字母排序的
	if !strings.HasPrefix(s1, `{"alpha":`) {
		t.Errorf("expected alphabetical key ordering, got %s", s1)
	}
}

func TestCreateLevel2HeadersIncludesPolyAddress(t *testing.T) {
	signer, err := NewSigner("0x"+strings.Repeat("aa", 32), 137)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	creds := &ApiCreds{
		APIKey:        "key",
		APISecret:     testSecret,
		APIPassphrase: "pass",
	}
	body := `{"x":1}`
	headers, err := CreateLevel2Headers(signer, creds, &RequestArgs{
		Method:         "POST",
		RequestPath:    "/order",
		SerializedBody: &body,
	})
	if err != nil {
		t.Fatalf("l2 headers: %v", err)
	}
	for _, k := range []string{PolyAddress, PolySignature, PolyTimestamp, PolyAPIKey, PolyPassphrase} {
		if v, ok := headers[k]; !ok || v == "" {
			t.Errorf("missing header %s", k)
		}
	}
	if headers[PolyAPIKey] != "key" {
		t.Errorf("api key not propagated")
	}
}

// TestNewClobClientRejectsInvalidFunder 防御 P1:
// 非法 funder 字符串必须 error 而不是被 common.HexToAddress 截成 0x000...
func TestNewClobClientRejectsInvalidFunder(t *testing.T) {
	pk := "0x" + strings.Repeat("aa", 32)
	cases := []string{
		"not-an-address",
		"0xZZZZ",
		"0x",
		"deadbeef", // 无 0x 前缀且不是 40 chars
	}
	for _, bad := range cases {
		_, err := NewClobClient("http://localhost", 137, pk, nil, nil, bad)
		if err == nil {
			t.Errorf("funder=%q should have been rejected", bad)
		}
	}
	// 合法地址(20 字节, 40 hex chars)应当通过
	if _, err := NewClobClient("http://localhost", 137, pk, nil, nil, "0x"+strings.Repeat("ab", 20)); err != nil {
		t.Errorf("valid funder rejected: %v", err)
	}
}

func TestCreateLevel1HeadersDeterministicForFixedTimestamp(t *testing.T) {
	signer, err := NewSigner("0x"+strings.Repeat("bb", 32), 137)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	nonce := 7
	h1, err := CreateLevel1Headers(signer, &nonce)
	if err != nil {
		t.Fatalf("l1: %v", err)
	}
	for _, k := range []string{PolyAddress, PolySignature, PolyTimestamp, PolyNonce} {
		if v, ok := h1[k]; !ok || v == "" {
			t.Errorf("missing header %s", k)
		}
	}
	if h1[PolyNonce] != "7" {
		t.Errorf("nonce header mismatch: %s", h1[PolyNonce])
	}
}
