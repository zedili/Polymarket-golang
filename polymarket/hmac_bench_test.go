package polymarket

import (
	"encoding/base64"
	"strings"
	"testing"
)

// 用于 benchmark 的真实大小 V2 payload。
var benchBody = strings.Repeat(`{"x":"y"}`, 60)
var benchSecret = func() string {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i)
	}
	return base64.URLEncoding.EncodeToString(raw)
}()

// BenchmarkBuildHMACSignature 旧路径:每次 base64 decode secret + HMAC + encode。
func BenchmarkBuildHMACSignature_Slow(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := BuildHMACSignature(benchSecret, 1700000000, "POST", "/order", benchBody)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBuildHMACSignatureRaw 新路径:secret 已预解码,跳过 base64 decode。
func BenchmarkBuildHMACSignature_Raw(b *testing.B) {
	decoded, _ := base64.URLEncoding.DecodeString(benchSecret)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := BuildHMACSignatureRaw(decoded, 1700000000, "POST", "/order", benchBody)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCreateLevel2Headers 模拟实际 L2 头生成成本(含 secret cache hit)。
func BenchmarkCreateLevel2Headers(b *testing.B) {
	signer, _ := NewSigner("0x"+strings.Repeat("ab", 32), 137)
	creds := &ApiCreds{APIKey: "k", APISecret: benchSecret, APIPassphrase: "p"}
	body := benchBody
	req := &RequestArgs{Method: "POST", RequestPath: "/order", SerializedBody: &body}
	// warm up secret cache
	_, _ = CreateLevel2Headers(signer, creds, req)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := CreateLevel2Headers(signer, creds, req)
		if err != nil {
			b.Fatal(err)
		}
	}
}
