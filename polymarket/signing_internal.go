package polymarket

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// SignClobAuthMessage 签名CLOB认证消息（L1认证）
// 使用EIP-712标准签名
func SignClobAuthMessage(signer *Signer, timestamp int, nonce int) (string, error) {
	// 构建EIP-712域
	// 根据EIP-712标准，域分隔符的构建方式：
	// keccak256(0x1901 || keccak256("EIP712Domain(string name,string version,uint256 chainId)") || nameHash || versionHash || chainId)
	domainNameHash := crypto.Keccak256Hash([]byte(CLOBDomainName))
	domainVersionHash := crypto.Keccak256Hash([]byte(CLOBVersion))
	chainID := big.NewInt(int64(signer.GetChainID()))

	// EIP712Domain类型哈希：keccak256("EIP712Domain(string name,string version,uint256 chainId)")
	eip712DomainTypeHash := crypto.Keccak256Hash([]byte("EIP712Domain(string name,string version,uint256 chainId)"))

	// 编码域数据
	domainData := make([]byte, 96) // 3个字段，每个32字节
	copy(domainData[0:32], domainNameHash[:])      // name hash
	copy(domainData[32:64], domainVersionHash[:])  // version hash
	copy(domainData[64:96], common.LeftPadBytes(chainID.Bytes(), 32)) // chainId

	// 构建域分隔符哈希
	domainHashBytes := append(eip712DomainTypeHash[:], domainData...)
	domainSeparator := crypto.Keccak256Hash(domainHashBytes)

	// 构建ClobAuth结构体
	address := common.HexToAddress(signer.Address())
	timestampStr := strconv.Itoa(timestamp)
	nonceBig := big.NewInt(int64(nonce))

	// EIP-712类型哈希：keccak256("ClobAuth(address address,string timestamp,uint256 nonce,string message)")
	// 注意：EIP-712规范要求类型字符串格式为 "type name"，必须包含字段名
	typeHash := crypto.Keccak256Hash([]byte("ClobAuth(address address,string timestamp,uint256 nonce,string message)"))

	// 根据EIP-712标准，字符串需要先进行keccak256哈希
	timestampHash := crypto.Keccak256Hash([]byte(timestampStr))
	messageStrHash := crypto.Keccak256Hash([]byte(MsgToSign))

	// 构建编码后的结构体值
	// address: 32字节（左对齐）
	// timestamp: 32字节（字符串的keccak256哈希）
	// nonce: 32字节（uint256）
	// message: 32字节（字符串的keccak256哈希）
	encoded := make([]byte, 128) // 4个字段，每个32字节
	
	// address (左对齐到32字节)
	copy(encoded[0:32], common.LeftPadBytes(address.Bytes(), 32))
	
	// timestamp hash (32字节)
	copy(encoded[32:64], timestampHash[:])
	
	// nonce (32字节，左对齐)
	copy(encoded[64:96], common.LeftPadBytes(nonceBig.Bytes(), 32))
	
	// message hash (32字节)
	copy(encoded[96:128], messageStrHash[:])

	// 构建结构体哈希：keccak256(typeHash || encodedValues)
	structHashBytes := append(typeHash[:], encoded...)
	structHash := crypto.Keccak256Hash(structHashBytes)

	// 构建signable_bytes（对应Python的signable_bytes方法）
	// 根据EIP-712标准和poly_eip712_structs的实现：
	// signable_bytes返回 "\x19\x01" || domainSeparator || structHash
	// 注意：domainSeparator本身已经是keccak256哈希，不需要再次哈希
	prefix := []byte("\x19\x01")
	signableBytes := append(prefix, domainSeparator[:]...)
	signableBytes = append(signableBytes, structHash.Bytes()...)

	// Python代码：keccak(signable_bytes).hex()
	// 对signable_bytes进行keccak256哈希
	authStructHash := crypto.Keccak256Hash(signableBytes)

	// Python代码：signer.sign(auth_struct_hash)
	// Account._sign_hash接收hex字符串，内部会解码为字节并签名
	// 我们直接对哈希值进行签名（等价于解码hex字符串后签名）
	signature, err := signer.Sign(authStructHash.Bytes())
	if err != nil {
		return "", err
	}

	// 添加0x前缀（如果还没有）
	if !strings.HasPrefix(signature, "0x") {
		signature = "0x" + signature
	}

	return signature, nil
}

// BuildHMACSignature 构建 HMAC 签名(L2 认证)。
//
// 关键约束:HMAC 签的字符串必须 byte-for-byte 等于实际发送给 API 的 body,
// 否则服务端会返回 401。本函数本身不会重新序列化对象,而是要求 body 已是
// 字符串(或 *string)。如果传入其他类型,本函数会调用 MarshalCompact 做一次
// 紧凑 JSON 序列化作为最后兜底,但调用方应该尽量自己提前序列化并把同一份
// 字符串同时用于 HMAC 与 HTTP body。
//
// 对照 py-clob-client-v2: build_hmac_signature(secret, ts, method, path, body)
// body 在 V2 中始终是预序列化的字符串(serialized_body),与本实现一致。
//
// 性能:secret 解码每次调用都跑;高频(机器人)场景下应通过 BuildHMACSignatureRaw
// 传入预解码的 []byte secret 来省掉 base64 decode。
func BuildHMACSignature(secret string, timestamp int, method string, requestPath string, body interface{}) (string, error) {
	base64Secret, err := base64.URLEncoding.DecodeString(secret)
	if err != nil {
		return "", fmt.Errorf("failed to decode secret: %w", err)
	}
	return BuildHMACSignatureRaw(base64Secret, timestamp, method, requestPath, body)
}

// BuildHMACSignatureRaw 是 BuildHMACSignature 的快速路径,跳过每次 base64 decode。
// 用 ApiCreds.DecodedSecret() 拿到预解码后的密钥。
func BuildHMACSignatureRaw(secretBytes []byte, timestamp int, method string, requestPath string, body interface{}) (string, error) {
	message := strconv.Itoa(timestamp) + method + requestPath
	if body != nil {
		bodyStr, err := normalizeBodyForHMAC(body)
		if err != nil {
			return "", err
		}
		message += bodyStr
	}
	mac := hmac.New(sha256.New, secretBytes)
	mac.Write([]byte(message))
	digest := mac.Sum(nil)
	return base64.URLEncoding.EncodeToString(digest), nil
}

// MarshalCompact 用紧凑 JSON 序列化任意 Go 值,等价于 Python
// json.dumps(x, separators=(",",":"), ensure_ascii=False)。
// 输出字符串既可用于 HMAC 签名,也可直接作为 HTTP body 发送。
//
// 注意:Go 的 json.Marshal 对 map 的 key 排序与 Python json.dumps 一致
// (按 key 字母排序),所以同一 Go map 多次序列化的结果是稳定的;但跨语言/
// 跨实现请仍以预序列化字符串为准。
func MarshalCompact(v interface{}) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal compact: %w", err)
	}
	return string(b), nil
}

// normalizeBodyForHMAC 是 BuildHMACSignature 内部 helper。
func normalizeBodyForHMAC(body interface{}) (string, error) {
	switch v := body.(type) {
	case string:
		return v, nil
	case *string:
		if v == nil {
			return "", nil
		}
		return *v, nil
	case []byte:
		return string(v), nil
	default:
		// 兜底序列化 —— 强烈建议调用方自己提前 marshal,以保证 HMAC 签的内容
		// 与 HTTP body 完全一致。
		return MarshalCompact(body)
	}
}

// CreateLevel1Headers 创建L1认证头
func CreateLevel1Headers(signer *Signer, nonce *int) (map[string]string, error) {
	timestamp := int(time.Now().Unix())

	n := 0
	if nonce != nil {
		n = *nonce
	}

	signature, err := SignClobAuthMessage(signer, timestamp, n)
	if err != nil {
		return nil, err
	}

	// 确保地址使用checksummed格式（与Python的eth_account一致）
	address := common.HexToAddress(signer.Address())
	
	headers := map[string]string{
		PolyAddress:   address.Hex(), // 使用checksummed格式
		PolySignature: signature,
		PolyTimestamp: strconv.Itoa(timestamp),
		PolyNonce:     strconv.Itoa(n),
	}

	// 调试输出（可以通过环境变量控制）
	if os.Getenv("DEBUG_HEADERS") == "1" {
		fmt.Fprintf(os.Stderr, "=== L1 Headers ===\n")
		for k, v := range headers {
			fmt.Fprintf(os.Stderr, "%s: %s\n", k, v)
		}
		fmt.Fprintf(os.Stderr, "==================\n")
	}

	return headers, nil
}

// CreateLevel2Headers 创建L2认证头
//
// 性能:走 ApiCreds.DecodedSecret() 取预解码后的 secret 字节(首次 decode、
// 后续 cache 命中),省掉每次签名的 base64 decode。
func CreateLevel2Headers(signer *Signer, creds *ApiCreds, requestArgs *RequestArgs) (map[string]string, error) {
	timestamp := int(time.Now().Unix())

	// 优先使用预序列化的body
	var bodyForSig interface{}
	if requestArgs.SerializedBody != nil {
		bodyForSig = *requestArgs.SerializedBody
	} else {
		bodyForSig = requestArgs.Body
	}

	secretBytes, err := creds.DecodedSecret()
	if err != nil {
		return nil, fmt.Errorf("failed to decode secret: %w", err)
	}

	hmacSig, err := BuildHMACSignatureRaw(
		secretBytes,
		timestamp,
		requestArgs.Method,
		requestArgs.RequestPath,
		bodyForSig,
	)
	if err != nil {
		return nil, err
	}

	return map[string]string{
		PolyAddress:    signer.Address(),
		PolySignature:  hmacSig,
		PolyTimestamp:  strconv.Itoa(timestamp),
		PolyAPIKey:     creds.APIKey,
		PolyPassphrase: creds.APIPassphrase,
	}, nil
}

// Builder header 常量
const (
	PolyBuilderAPIKey     = "POLY_BUILDER_API_KEY"
	PolyBuilderPassphrase = "POLY_BUILDER_PASSPHRASE"
	PolyBuilderSignature  = "POLY_BUILDER_SIGNATURE"
	PolyBuilderTimestamp  = "POLY_BUILDER_TIMESTAMP"
)

// CreateBuilderHeaders 创建 Builder 认证头（用于 Gasless 交易）
func CreateBuilderHeaders(creds *ApiCreds, requestArgs *RequestArgs) (map[string]string, error) {
	timestamp := int(time.Now().Unix())

	// 优先使用预序列化的body
	var bodyForSig interface{}
	if requestArgs.SerializedBody != nil {
		bodyForSig = *requestArgs.SerializedBody
	} else {
		bodyForSig = requestArgs.Body
	}

	hmacSig, err := BuildHMACSignature(
		creds.APISecret,
		timestamp,
		requestArgs.Method,
		requestArgs.RequestPath,
		bodyForSig,
	)
	if err != nil {
		return nil, err
	}

	return map[string]string{
		PolyBuilderSignature:  hmacSig,
		PolyBuilderTimestamp:  strconv.Itoa(timestamp),
		PolyBuilderAPIKey:     creds.APIKey,
		PolyBuilderPassphrase: creds.APIPassphrase,
	}, nil
}

