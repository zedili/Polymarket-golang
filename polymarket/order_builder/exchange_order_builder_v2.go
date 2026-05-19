package order_builder

import (
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// ============================================================
//  V2 EIP-712 类型/常量
// ============================================================

// V2 Order 结构(对应 py-clob-client-v2 CTF_EXCHANGE_V2_ORDER_STRUCT)。
// 注意:Order 类型 hash 内不包含 expiration —— V2 把 expiration 仅放在 API
// payload 里,不参与签名。
const v2OrderTypeString = "Order(uint256 salt,address maker,address signer,uint256 tokenId,uint256 makerAmount,uint256 takerAmount,uint8 side,uint8 signatureType,uint256 timestamp,bytes32 metadata,bytes32 builder)"

// Solady TypedDataSign 包装(用于 POLY_1271 智能合约钱包签名)。
const v2SoladyTypeString = "TypedDataSign(Order contents,string name,string version,uint256 chainId,address verifyingContract,bytes32 salt)" + v2OrderTypeString

const v2EIP712DomainTypeString = "EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"

// V2 域常量
const (
	v2CTFExchangeDomainName    = "Polymarket CTF Exchange"
	v2CTFExchangeDomainVersion = "2"
	v2DepositWalletDomainName  = "DepositWallet"
	v2DepositWalletVersion     = "1"
)

// V2 签名类型
const (
	V2SigTypeEOA            = 0
	V2SigTypePolyProxy      = 1
	V2SigTypePolyGnosisSafe = 2
	V2SigTypePoly1271       = 3
)

// V2 订单的 Side
const (
	V2SideBuy  = 0
	V2SideSell = 1
)

// 预计算的常量 hash
var (
	v2OrderTypeHash             = crypto.Keccak256Hash([]byte(v2OrderTypeString))
	v2SoladyTypeHash            = crypto.Keccak256Hash([]byte(v2SoladyTypeString))
	v2DomainTypeHash            = crypto.Keccak256Hash([]byte(v2EIP712DomainTypeString))
	v2CTFExchangeNameHash       = crypto.Keccak256Hash([]byte(v2CTFExchangeDomainName))
	v2CTFExchangeVersionHash    = crypto.Keccak256Hash([]byte(v2CTFExchangeDomainVersion))
	v2DepositWalletNameHash     = crypto.Keccak256Hash([]byte(v2DepositWalletDomainName))
	v2DepositWalletVersionHash  = crypto.Keccak256Hash([]byte(v2DepositWalletVersion))
	v2DepositWalletDomainSalt32 = [32]byte{} // bytes32 全 0
)

// ABI 类型(包内只初始化一次)
var (
	v2TypeUint256, _ = abi.NewType("uint256", "", nil)
	v2TypeUint8, _   = abi.NewType("uint8", "", nil)
	v2TypeAddress, _ = abi.NewType("address", "", nil)
	v2TypeBytes32, _ = abi.NewType("bytes32", "", nil)
)

// ============================================================
//  OrderDataV2 / OrderV2 / SignedOrderV2 (本包内副本,避免循环依赖)
// ============================================================

// OrderDataV2 V2 订单输入(用于 build_signed_order)。
type OrderDataV2 struct {
	Maker         string
	TokenID       string
	MakerAmount   string
	TakerAmount   string
	Side          int    // 0 BUY, 1 SELL
	Signer        string // 可选,默认 = maker
	SignatureType int
	Timestamp     string // 毫秒;空表示自动生成
	Metadata      string // bytes32 hex
	Builder       string // bytes32 hex
	Expiration    string // 默认 "0"
}

// OrderV2 已构造但未签名的 V2 订单。
type OrderV2 struct {
	Salt          *big.Int
	Maker         common.Address
	Signer        common.Address
	TokenID       *big.Int
	MakerAmount   *big.Int
	TakerAmount   *big.Int
	Side          uint8
	SignatureType uint8
	Timestamp     *big.Int
	Metadata      [32]byte
	Builder       [32]byte
	Expiration    *big.Int // 不参与签名,仅在 JSON 输出时使用
}

// SignedOrderV2 已签名的 V2 订单。
type SignedOrderV2 struct {
	OrderV2
	Signature string // "0x..." hex
}

// ============================================================
//  ExchangeOrderBuilderV2
// ============================================================

// ExchangeOrderBuilderV2 构造并签名 V2 订单。
type ExchangeOrderBuilderV2 struct {
	contractAddress   common.Address
	chainID           *big.Int
	privateKey        *ecdsa.PrivateKey
	signerAddress     common.Address
	appDomainSeparator [32]byte
	saltGenerator     func() *big.Int
}

// NewExchangeOrderBuilderV2 创建 V2 builder。
//
//   contractAddress: V2 Exchange 合约地址(根据 negRisk 选 exchange_v2 或
//                    neg_risk_exchange_v2)。
//   chainID:        Polygon=137, Amoy=80002
//   privateKey:     用于 EOA 签名的私钥
//   saltGenerator:  可选,nil 时使用默认随机 salt 生成器
func NewExchangeOrderBuilderV2(
	contractAddress string,
	chainID int,
	privateKey *ecdsa.PrivateKey,
	saltGenerator func() *big.Int,
) (*ExchangeOrderBuilderV2, error) {
	if privateKey == nil {
		return nil, fmt.Errorf("private key is required")
	}
	if !common.IsHexAddress(contractAddress) {
		return nil, fmt.Errorf("invalid contract address: %s", contractAddress)
	}

	pubKey, ok := privateKey.Public().(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("invalid public key from private key")
	}
	signerAddr := crypto.PubkeyToAddress(*pubKey)

	contractAddr := common.HexToAddress(contractAddress)
	chainIDBig := big.NewInt(int64(chainID))

	appDomainSep, err := computeAppDomainSeparator(chainIDBig, contractAddr)
	if err != nil {
		return nil, err
	}

	if saltGenerator == nil {
		saltGenerator = defaultSaltGenerator
	}

	return &ExchangeOrderBuilderV2{
		contractAddress:    contractAddr,
		chainID:            chainIDBig,
		privateKey:         privateKey,
		signerAddress:      signerAddr,
		appDomainSeparator: appDomainSep,
		saltGenerator:      saltGenerator,
	}, nil
}

// SignerAddress 返回签名者(EOA)地址。
func (b *ExchangeOrderBuilderV2) SignerAddress() common.Address {
	return b.signerAddress
}

// AppDomainSeparator 返回 V2 CTF Exchange 域分隔符(用于 POLY_1271 拼接)。
func (b *ExchangeOrderBuilderV2) AppDomainSeparator() [32]byte {
	return b.appDomainSeparator
}

// ChainID 返回 chain id。
func (b *ExchangeOrderBuilderV2) ChainID() *big.Int {
	return new(big.Int).Set(b.chainID)
}

// ContractAddress 返回 V2 Exchange 合约地址。
func (b *ExchangeOrderBuilderV2) ContractAddress() common.Address {
	return b.contractAddress
}

// BuildOrder 把 OrderDataV2 转成可签名的 OrderV2(填充 salt / timestamp 等默认值)。
func (b *ExchangeOrderBuilderV2) BuildOrder(data *OrderDataV2) (*OrderV2, error) {
	if data == nil {
		return nil, fmt.Errorf("order data is nil")
	}

	// 显式校验所有地址,防止 common.HexToAddress 把非法字符串静默转为零地址,
	// 进而产生"maker / signer = 0x000..."的废订单。
	if !common.IsHexAddress(data.Maker) {
		return nil, fmt.Errorf("invalid maker address: %q", data.Maker)
	}
	signerStr := data.Signer
	if signerStr == "" {
		signerStr = data.Maker
	}
	if !common.IsHexAddress(signerStr) {
		return nil, fmt.Errorf("invalid signer address: %q", signerStr)
	}

	sigType := uint8(data.SignatureType)
	// 只有纯 EOA 路径要求 signer == 签名者 EOA。
	// PolyProxy / GnosisSafe / POLY_1271 路径下,signer 可以是 SCA 钱包地址,
	// 由 SCA 通过 isValidSignature(EIP-1271)把签名 delegate 给 owner EOA。
	if sigType == V2SigTypeEOA {
		if !strings.EqualFold(signerStr, b.signerAddress.Hex()) {
			return nil, fmt.Errorf("signer does not match: %s vs %s", signerStr, b.signerAddress.Hex())
		}
	}

	timestamp := data.Timestamp
	if timestamp == "" {
		timestamp = fmt.Sprintf("%d", time.Now().UnixNano()/1_000_000)
	}

	metadata := data.Metadata
	if metadata == "" {
		metadata = bytes32Zero
	}
	builderCode := data.Builder
	if builderCode == "" {
		builderCode = bytes32Zero
	}
	expiration := data.Expiration
	if expiration == "" {
		expiration = "0"
	}

	tokenID, ok := new(big.Int).SetString(data.TokenID, 10)
	if !ok {
		return nil, fmt.Errorf("invalid tokenID: %s", data.TokenID)
	}
	makerAmt, ok := new(big.Int).SetString(data.MakerAmount, 10)
	if !ok {
		return nil, fmt.Errorf("invalid makerAmount: %s", data.MakerAmount)
	}
	takerAmt, ok := new(big.Int).SetString(data.TakerAmount, 10)
	if !ok {
		return nil, fmt.Errorf("invalid takerAmount: %s", data.TakerAmount)
	}
	tsBig, ok := new(big.Int).SetString(timestamp, 10)
	if !ok {
		return nil, fmt.Errorf("invalid timestamp: %s", timestamp)
	}
	expBig, ok := new(big.Int).SetString(expiration, 10)
	if !ok {
		return nil, fmt.Errorf("invalid expiration: %s", expiration)
	}

	metaBytes, err := hexTo32Bytes(metadata)
	if err != nil {
		return nil, fmt.Errorf("invalid metadata: %w", err)
	}
	builderBytes, err := hexTo32Bytes(builderCode)
	if err != nil {
		return nil, fmt.Errorf("invalid builder: %w", err)
	}

	salt := b.saltGenerator()

	return &OrderV2{
		Salt:          salt,
		Maker:         common.HexToAddress(data.Maker),
		Signer:        common.HexToAddress(signerStr),
		TokenID:       tokenID,
		MakerAmount:   makerAmt,
		TakerAmount:   takerAmt,
		Side:          uint8(data.Side),
		SignatureType: sigType,
		Timestamp:     tsBig,
		Metadata:      metaBytes,
		Builder:       builderBytes,
		Expiration:    expBig,
	}, nil
}

// OrderTypedDataHash 计算 V2 订单的 EIP-712 typed data hash:
//   keccak256("\x19\x01" || appDomainSeparator || hashStruct(order))
//
// 该 hash 是 EOA / Proxy / GnosisSafe 签名时实际签的内容。
func (b *ExchangeOrderBuilderV2) OrderTypedDataHash(order *OrderV2) ([32]byte, error) {
	contentsHash, err := hashOrderV2Struct(order)
	if err != nil {
		return [32]byte{}, err
	}
	return finalEIP712Digest(b.appDomainSeparator, contentsHash), nil
}

// ContentsHash 暴露 hashStruct(Order) —— POLY_1271 路径需要。
func (b *ExchangeOrderBuilderV2) ContentsHash(order *OrderV2) ([32]byte, error) {
	return hashOrderV2Struct(order)
}

// SignOrder 签名 V2 订单,根据 signatureType 走 EOA 或 POLY_1271 路径。
func (b *ExchangeOrderBuilderV2) SignOrder(order *OrderV2) (string, error) {
	if order.SignatureType == V2SigTypePoly1271 {
		return b.signPoly1271(order)
	}
	digest, err := b.OrderTypedDataHash(order)
	if err != nil {
		return "", err
	}
	return signDigest(b.privateKey, digest[:])
}

// BuildSignedOrder 构造 + 签名一气呵成。
func (b *ExchangeOrderBuilderV2) BuildSignedOrder(data *OrderDataV2) (*SignedOrderV2, error) {
	order, err := b.BuildOrder(data)
	if err != nil {
		return nil, err
	}
	sig, err := b.SignOrder(order)
	if err != nil {
		return nil, err
	}
	return &SignedOrderV2{OrderV2: *order, Signature: sig}, nil
}

// ============================================================
//  POLY_1271 (Solady TypedDataSign) 签名
// ============================================================

// signPoly1271 实现 EIP-1271 + Solady TypedDataSign 包装:
//
//   1. contentsHash = hashStruct(Order)
//   2. typedDataSignStructHash = keccak256(abi.encode(
//        SOLADY_TYPE_HASH,
//        contentsHash,
//        DEPOSIT_WALLET_NAME_HASH,
//        DEPOSIT_WALLET_VERSION_HASH,
//        chainId,
//        order.signer,            // 即 Deposit Wallet 合约地址(作为 verifyingContract)
//        DEPOSIT_WALLET_DOMAIN_SALT,
//      ))
//   3. digest = keccak256("\x19\x01" || appDomainSeparator || typedDataSignStructHash)
//   4. inner = ECDSA(digest)
//   5. signature = inner(65 bytes) || appDomainSeparator(32) || contentsHash(32)
//                  || contentsTypeString || contentsTypeLen(uint16 BE)
func (b *ExchangeOrderBuilderV2) signPoly1271(order *OrderV2) (string, error) {
	contentsHash, err := hashOrderV2Struct(order)
	if err != nil {
		return "", err
	}

	args := abi.Arguments{
		{Type: v2TypeBytes32},
		{Type: v2TypeBytes32},
		{Type: v2TypeBytes32},
		{Type: v2TypeBytes32},
		{Type: v2TypeUint256},
		{Type: v2TypeAddress},
		{Type: v2TypeBytes32},
	}
	encoded, err := args.Pack(
		[32]byte(v2SoladyTypeHash),
		contentsHash,
		[32]byte(v2DepositWalletNameHash),
		[32]byte(v2DepositWalletVersionHash),
		b.chainID,
		order.Signer,
		v2DepositWalletDomainSalt32,
	)
	if err != nil {
		return "", fmt.Errorf("pack solady type: %w", err)
	}
	typedDataSignStructHash := crypto.Keccak256Hash(encoded)

	digest := finalEIP712Digest(b.appDomainSeparator, [32]byte(typedDataSignStructHash))

	innerSig, err := crypto.Sign(digest[:], b.privateKey)
	if err != nil {
		return "", fmt.Errorf("ecdsa sign: %w", err)
	}
	// 与 EOA 路径保持一致 (v += 27)
	innerSig[64] += 27

	contentsType := []byte(v2OrderTypeString)
	contentsTypeLen := make([]byte, 2)
	binary.BigEndian.PutUint16(contentsTypeLen, uint16(len(contentsType)))

	out := make([]byte, 0, len(innerSig)+32+32+len(contentsType)+2)
	out = append(out, innerSig...)
	out = append(out, b.appDomainSeparator[:]...)
	out = append(out, contentsHash[:]...)
	out = append(out, contentsType...)
	out = append(out, contentsTypeLen...)

	return "0x" + hex.EncodeToString(out), nil
}

// ============================================================
//  Helpers
// ============================================================

const bytes32Zero = "0x0000000000000000000000000000000000000000000000000000000000000000"

// ============================================================
//  V2 订单金额计算(对齐 py-clob-client-v2 builder.py)
// ============================================================

// GetOrderAmountsV2 V2 限价订单 maker/taker 金额。
// 与 V1 一致:price 用 RoundNormal。
// 返回 (side, makerAmount, takerAmount, error),side 0=BUY,1=SELL。
func GetOrderAmountsV2(side string, size, price float64, cfg RoundConfig) (uint8, *big.Int, *big.Int, error) {
	return getAmountsV2(side, size, price, cfg, false)
}

// GetMarketOrderAmountsV2 V2 市价订单 maker/taker 金额。
// **关键差异**:price 用 RoundDown(V1 用 RoundNormal),与 py-clob-client-v2
// builder.py:104 一致 ("V2 change: market orders use round_down for price")。
func GetMarketOrderAmountsV2(side string, amount, price float64, cfg RoundConfig) (uint8, *big.Int, *big.Int, error) {
	return getAmountsV2(side, amount, price, cfg, true)
}

func getAmountsV2(side string, sizeOrAmount, price float64, cfg RoundConfig, isMarket bool) (uint8, *big.Int, *big.Int, error) {
	var rawPrice float64
	if isMarket {
		rawPrice = RoundDown(price, cfg.Price)
	} else {
		rawPrice = RoundNormal(price, cfg.Price)
	}

	switch side {
	case "BUY":
		var rawMaker, rawTaker float64
		if isMarket {
			// BUY 市价:amount 是 USDC 金额,maker=USDC,taker=tokens=maker/price
			rawMaker = RoundDown(sizeOrAmount, cfg.Size)
			rawTaker = rawMaker / rawPrice
		} else {
			// BUY 限价:size 是 tokens,taker=tokens=round_down(size),maker=USDC=taker*price
			rawTaker = RoundDown(sizeOrAmount, cfg.Size)
			rawMaker = rawTaker * rawPrice
		}
		// 与 Python 一致:若 maker(限价)/taker(市价)小数位超出 amount 精度,
		// 先 round_up(amount+4),再 round_down(amount)。
		toCheck := &rawMaker
		if isMarket {
			toCheck = &rawTaker
		}
		if DecimalPlaces(*toCheck) > cfg.Amount {
			*toCheck = RoundUp(*toCheck, cfg.Amount+4)
			if DecimalPlaces(*toCheck) > cfg.Amount {
				*toCheck = RoundDown(*toCheck, cfg.Amount)
			}
		}
		return 0, big.NewInt(ToTokenDecimals(rawMaker)), big.NewInt(ToTokenDecimals(rawTaker)), nil

	case "SELL":
		// SELL(限价 + 市价同):size/amount 是 tokens,maker=tokens=round_down,taker=USDC=maker*price
		rawMaker := RoundDown(sizeOrAmount, cfg.Size)
		rawTaker := rawMaker * rawPrice
		if DecimalPlaces(rawTaker) > cfg.Amount {
			rawTaker = RoundUp(rawTaker, cfg.Amount+4)
			if DecimalPlaces(rawTaker) > cfg.Amount {
				rawTaker = RoundDown(rawTaker, cfg.Amount)
			}
		}
		return 1, big.NewInt(ToTokenDecimals(rawMaker)), big.NewInt(ToTokenDecimals(rawTaker)), nil
	}

	return 0, nil, nil, fmt.Errorf("side must be BUY or SELL, got %q", side)
}

// hashOrderV2Struct 计算 EIP-712 Order 类型的 hashStruct(),即:
//   keccak256(abi.encode(ORDER_TYPE_HASH, salt, maker, signer, tokenId,
//                        makerAmount, takerAmount, side, signatureType,
//                        timestamp, metadata, builder))
func hashOrderV2Struct(o *OrderV2) ([32]byte, error) {
	args := abi.Arguments{
		{Type: v2TypeBytes32}, // ORDER_TYPE_HASH
		{Type: v2TypeUint256}, // salt
		{Type: v2TypeAddress}, // maker
		{Type: v2TypeAddress}, // signer
		{Type: v2TypeUint256}, // tokenId
		{Type: v2TypeUint256}, // makerAmount
		{Type: v2TypeUint256}, // takerAmount
		{Type: v2TypeUint8},   // side
		{Type: v2TypeUint8},   // signatureType
		{Type: v2TypeUint256}, // timestamp
		{Type: v2TypeBytes32}, // metadata
		{Type: v2TypeBytes32}, // builder
	}
	encoded, err := args.Pack(
		[32]byte(v2OrderTypeHash),
		o.Salt,
		o.Maker,
		o.Signer,
		o.TokenID,
		o.MakerAmount,
		o.TakerAmount,
		o.Side,
		o.SignatureType,
		o.Timestamp,
		o.Metadata,
		o.Builder,
	)
	if err != nil {
		return [32]byte{}, fmt.Errorf("pack order struct: %w", err)
	}
	return [32]byte(crypto.Keccak256Hash(encoded)), nil
}

// computeAppDomainSeparator 计算 V2 CTF Exchange 的 EIP-712 domain separator:
//   keccak256(abi.encode(
//     EIP712_DOMAIN_TYPE_HASH,
//     keccak256("Polymarket CTF Exchange"),
//     keccak256("2"),
//     chainId,
//     verifyingContract,
//   ))
func computeAppDomainSeparator(chainID *big.Int, contract common.Address) ([32]byte, error) {
	args := abi.Arguments{
		{Type: v2TypeBytes32},
		{Type: v2TypeBytes32},
		{Type: v2TypeBytes32},
		{Type: v2TypeUint256},
		{Type: v2TypeAddress},
	}
	encoded, err := args.Pack(
		[32]byte(v2DomainTypeHash),
		[32]byte(v2CTFExchangeNameHash),
		[32]byte(v2CTFExchangeVersionHash),
		chainID,
		contract,
	)
	if err != nil {
		return [32]byte{}, fmt.Errorf("pack domain: %w", err)
	}
	return [32]byte(crypto.Keccak256Hash(encoded)), nil
}

// finalEIP712Digest = keccak256("\x19\x01" || domainSep || structHash)
func finalEIP712Digest(domainSep, structHash [32]byte) [32]byte {
	buf := make([]byte, 0, 2+32+32)
	buf = append(buf, 0x19, 0x01)
	buf = append(buf, domainSep[:]...)
	buf = append(buf, structHash[:]...)
	return [32]byte(crypto.Keccak256Hash(buf))
}

// signDigest 对已经准备好的 32 字节 digest 用 ECDSA 签名,
// 输出 "0x" 前缀的 65 字节签名(v = 27/28)。
func signDigest(pk *ecdsa.PrivateKey, digest []byte) (string, error) {
	if len(digest) != 32 {
		return "", fmt.Errorf("digest must be 32 bytes")
	}
	sig, err := crypto.Sign(digest, pk)
	if err != nil {
		return "", fmt.Errorf("ecdsa sign: %w", err)
	}
	sig[64] += 27
	return "0x" + hex.EncodeToString(sig), nil
}

// hexTo32Bytes 接受 "0x..." / "..." hex,左 pad 到 32 字节。
func hexTo32Bytes(s string) ([32]byte, error) {
	var out [32]byte
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")
	if len(s) == 0 {
		return out, nil
	}
	if len(s)%2 == 1 {
		s = "0" + s
	}
	raw, err := hex.DecodeString(s)
	if err != nil {
		return out, err
	}
	if len(raw) > 32 {
		return out, fmt.Errorf("hex value > 32 bytes: %s", s)
	}
	copy(out[32-len(raw):], raw)
	return out, nil
}

// defaultSaltGenerator 生成 [0, 2^32) 区间的 salt(与 V1 一致)。
func defaultSaltGenerator() *big.Int {
	max := new(big.Int).Lsh(big.NewInt(1), 32)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		// 极端情况下退化到时间戳
		return big.NewInt(time.Now().UnixNano())
	}
	return n
}

// ============================================================
//  JSON 编码:与 py-clob-client-v2 order_to_json_v2 等价
// ============================================================

// ToJSONPayload 将已签名 V2 订单转成 POST /order 期望的 JSON payload(map)。
// 字段顺序与 py-clob-client-v2 一致,便于阅读和服务端对账。
//
// salt 用 *big.Int 直接放进 map —— Go 的 json.Marshal 通过 *big.Int.MarshalJSON
// 输出无引号十进制数字(等同 Python `int(order.salt)`),不会因外部 saltGenerator
// 返回 > int64.MaxValue 的 uint256 而截断。
func (o *SignedOrderV2) ToJSONPayload(owner string, orderType string, postOnly, deferExec bool) map[string]interface{} {
	side := "BUY"
	if o.Side == V2SideSell {
		side = "SELL"
	}
	return map[string]interface{}{
		"order": map[string]interface{}{
			"salt":          new(big.Int).Set(o.Salt),
			"maker":         o.Maker.Hex(),
			"signer":        o.Signer.Hex(),
			"tokenId":       o.TokenID.String(),
			"makerAmount":   o.MakerAmount.String(),
			"takerAmount":   o.TakerAmount.String(),
			"side":          side,
			"expiration":    o.Expiration.String(),
			"signatureType": int(o.SignatureType),
			"timestamp":     o.Timestamp.String(),
			"metadata":      "0x" + hex.EncodeToString(o.Metadata[:]),
			"builder":       "0x" + hex.EncodeToString(o.Builder[:]),
			"signature":     o.Signature,
		},
		"owner":     owner,
		"orderType": orderType,
		"deferExec": deferExec,
		"postOnly":  postOnly,
	}
}
