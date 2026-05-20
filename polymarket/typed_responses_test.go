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
			"id":             "0xc976a32babf0b64801cda48704054b1772624f76ded1c7ac88fa29ad7f1844cb",
			"status":         "LIVE",
			"owner":          "77fb58cb-362f-51b3-8c8c-7e65d8583637",
			"maker_address":  "0xc16bDe710291149EFAd2e8bDaeB7FF9fCeB22B55",
			"market":         "0x384e2707bbb95da4bfa6f330fe7d5ccbec1c0a85e20be900cbf599987588e1a4",
			"asset_id":       "78433024518676680431174478322854148606578065650008220678402966840627347604025",
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
		ID:           "0xc976a32babf0b64801cda48704054b1772624f76ded1c7ac88fa29ad7f1844cb",
		Status:       "LIVE",
		Owner:        "77fb58cb-362f-51b3-8c8c-7e65d8583637",
		MakerAddress: "0xc16bDe710291149EFAd2e8bDaeB7FF9fCeB22B55",
		Market:       "0x384e2707bbb95da4bfa6f330fe7d5ccbec1c0a85e20be900cbf599987588e1a4",
		AssetID:      "78433024518676680431174478322854148606578065650008220678402966840627347604025",
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
