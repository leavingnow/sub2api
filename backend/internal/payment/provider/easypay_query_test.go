package provider

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func assertPayFMQuery(t *testing.T, r *http.Request, merchant, order, key string) {
	t.Helper()
	if r.Method != http.MethodPost || r.URL.Path != "/queryOutOrder" {
		t.Errorf("query request = %s %s, want POST /queryOutOrder", r.Method, r.URL.Path)
	}
	q := r.URL.Query()
	digest := md5.Sum([]byte(merchant + order + key))
	if len(q) != 3 || q.Get("merchantNum") != merchant || q.Get("orderNo") != order || q.Get("sign") != hex.EncodeToString(digest[:]) {
		t.Errorf("unexpected PayFM query parameters: %v", q)
	}
}

func TestEasyPayQueryOrderStatusMapping(t *testing.T) {
	t.Parallel()
	const orderID = "order-123"

	tests := []struct {
		name       string
		body       string
		wantStatus string
		wantErr    string
	}{
		{
			name:       "paid",
			body:       `{"success":true,"code":200,"data":{"merchantNum":"pid-1","orderNo":"order-123","amount":"12.34","orderState":4}}`,
			wantStatus: payment.ProviderStatusPaid,
		},
		{
			name:       "pending with string state",
			body:       `{"success":true,"code":"200","data":{"merchantNum":"pid-1","orderNo":"order-123","amount":12.34,"orderState":"2"}}`,
			wantStatus: payment.ProviderStatusPending,
		},
		{
			name:       "failed",
			body:       `{"success":true,"code":200,"data":{"merchantNum":"pid-1","orderNo":"order-123","amount":"12.34","orderState":7}}`,
			wantStatus: payment.ProviderStatusFailed,
		},
		{
			name:    "vendor error",
			body:    `{"success":false,"code":400,"msg":"订单不存在"}`,
			wantErr: "订单不存在",
		},
		{
			name:    "unknown state",
			body:    `{"success":true,"code":200,"data":{"merchantNum":"pid-1","orderNo":"order-123","amount":"12.34","orderState":99}}`,
			wantErr: "unknown order state",
		},
		{
			name:    "invalid amount",
			body:    `{"success":true,"code":200,"data":{"merchantNum":"pid-1","orderNo":"order-123","amount":"NaN","orderState":4}}`,
			wantErr: "invalid amount",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assertPayFMQuery(t, r, "pid-1", orderID, "pkey-1")
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			p := newTestEasyPay(t, server.URL)
			resp, err := p.QueryOrder(context.Background(), orderID)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("QueryOrder error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("QueryOrder: %v", err)
			}
			if resp.TradeNo != orderID || resp.Status != tt.wantStatus || resp.Amount != 12.34 || resp.Metadata["pid"] != "pid-1" {
				t.Fatalf("unexpected order response: %+v", resp)
			}
		})
	}
}
