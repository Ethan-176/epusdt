package addressutil

import "testing"

func TestNormalizeWalletAddress(t *testing.T) {
	tests := []struct {
		name    string
		network string
		input   string
		want    string
		wantErr bool
	}{
		{name: "tron", network: NetworkTron, input: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", want: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"},
		{name: "evm lowercase", network: NetworkEthereum, input: "0xA1B2c3D4e5F60718293aBcDeF001122334455667", want: "0xa1b2c3d4e5f60718293abcdef001122334455667"},
		{name: "invalid evm", network: NetworkEthereum, input: "0x123", wantErr: true},
		{name: "unsupported", network: "bitcoin", input: "anything", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeWalletAddress(tt.network, tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NormalizeWalletAddress() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeWalletAddress(): %v", err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeWalletAddress() = %q, want %q", got, tt.want)
			}
		})
	}
}
