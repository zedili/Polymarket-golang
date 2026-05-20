package polymarket

import (
	"reflect"
	"testing"
)

// TestDecodeOrdersList 验证 GetOrdersTyped 的 JSON 桥接逻辑能正确转换。
// 用一组真实 polymarket 响应 shape 做 fixture。
func TestDecodeOrdersList(t *testing.T) {
	// 真实 V2 服务端的 OpenOrder 响应字段(从 docs.polymarket.com /api-reference/trade/get-user-orders)
	raw := []interface{}{
		map[string]interface{}{
			"id":             "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			"status":         "LIVE",
			"owner":          "00000000-0000-0000-0000-000000000000",
			"maker_address":  "0x1111111111111111111111111111111111111111",
			"market":         "0xfedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321",
			"asset_id":       "12345678901234567890123456789012345678901234567890123456789012345678901234567",
			"side":           "SELL",
			"original_size":  "4.5454",
			"size_matched":   "0",
			"price":          "0.99",
			"outcome":        "Yes",
			"expiration":     "0",
			"order_type":     "GTC",
			"created_at":     float64(1779216600), // JSON numbers come as float64
		},
	}

	got, err := decodeOrdersList(raw)
	if err != nil {
		t.Fatalf("decodeOrdersList: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 order, got %d", len(got))
	}
	want := OpenOrder{
		ID:           "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		Status:       "LIVE",
		Owner:        "00000000-0000-0000-0000-000000000000",
		MakerAddress: "0x1111111111111111111111111111111111111111",
		Market:       "0xfedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321",
		AssetID:      "12345678901234567890123456789012345678901234567890123456789012345678901234567",
		Side:         "SELL",
		OriginalSize: "4.5454",
		SizeMatched:  "0",
		Price:        "0.99",
		Outcome:      "Yes",
		Expiration:   "0",
		OrderType:    "GTC",
		CreatedAt:    1779216600,
	}
	if !reflect.DeepEqual(got[0], want) {
		t.Errorf("\n got: %+v\nwant: %+v", got[0], want)
	}
}

// TestDecodeCancelResult 验证 Cancel* 响应的两种 shape:object 和 array。
func TestDecodeCancelResult(t *testing.T) {
	// shape 1: object (新 V2)
	raw1 := map[string]interface{}{
		"canceled":     []interface{}{"0xabc", "0xdef"},
		"not_canceled": map[string]interface{}{"0xghi": "ORDER_NOT_FOUND"},
	}
	r, err := decodeCancel(raw1)
	if err != nil {
		t.Fatalf("decodeCancel(object): %v", err)
	}
	if !reflect.DeepEqual(r.Canceled, []string{"0xabc", "0xdef"}) {
		t.Errorf("canceled mismatch: %v", r.Canceled)
	}
	if r.NotCanceled["0xghi"] != "ORDER_NOT_FOUND" {
		t.Errorf("not_canceled mismatch: %v", r.NotCanceled)
	}

	// shape 2: array (老 V1 fallback)
	raw2 := []interface{}{"0xaaa", "0xbbb"}
	r2, err := decodeCancel(raw2)
	if err != nil {
		t.Fatalf("decodeCancel(array): %v", err)
	}
	if !reflect.DeepEqual(r2.Canceled, []string{"0xaaa", "0xbbb"}) {
		t.Errorf("array→canceled mismatch: %v", r2.Canceled)
	}

	// shape 3: nil(意外)
	r3, err := decodeCancel(nil)
	if err != nil {
		t.Errorf("decodeCancel(nil) should not error: %v", err)
	}
	if r3 == nil || len(r3.Canceled) != 0 {
		t.Errorf("nil → empty CancelResult expected, got %v", r3)
	}
}

// TestJSONRoundtrip 验证 jsonRoundtrip 不丢字段、不增字段。
func TestJSONRoundtrip(t *testing.T) {
	src := map[string]interface{}{
		"id":           "x",
		"asset_id":     "123",
		"side":         "BUY",
		"size":         "1.5",
		"price":        "0.5",
		"transaction_hash": "0xdead",
	}
	var dst Trade
	if err := jsonRoundtrip(src, &dst); err != nil {
		t.Fatalf("jsonRoundtrip: %v", err)
	}
	if dst.ID != "x" || dst.AssetID != "123" || dst.Side != "BUY" || dst.Size != "1.5" ||
		dst.Price != "0.5" || dst.TransactionHash != "0xdead" {
		t.Errorf("decoded Trade mismatch: %+v", dst)
	}
}
