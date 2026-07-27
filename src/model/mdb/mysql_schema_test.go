package mdb

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func TestMySQLIndexedStringFieldsHaveBoundedSizes(t *testing.T) {
	tests := []struct {
		model  interface{}
		fields map[string]int
	}{
		{
			model: &Orders{},
			fields: map[string]int{
				"TradeId":            32,
				"OrderId":            32,
				"ParentTradeId":      32,
				"BlockTransactionId": 191,
			},
		},
		{
			model: &WalletAddress{},
			fields: map[string]int{
				"Network": 32,
				"Address": 128,
			},
		},
	}

	for _, tt := range tests {
		parsed, err := schema.Parse(tt.model, &sync.Map{}, schema.NamingStrategy{})
		if err != nil {
			t.Fatalf("parse schema for %T: %v", tt.model, err)
		}
		for name, want := range tt.fields {
			field := parsed.LookUpField(name)
			if field == nil {
				t.Fatalf("%T field %s not found", tt.model, name)
			}
			if field.Size != want {
				t.Errorf("%T.%s size = %d, want %d", tt.model, name, field.Size, want)
			}
		}
	}
}
