package main

import "testing"

func TestExplicitClientProfile(t *testing.T) {
	for _, test := range []struct {
		endpoint, profile string
		ok                bool
	}{
		{"http://127.0.0.1:26657", "local", true},
		{"https://rpc.example", "local", false},
		{"http://rpc.example:26657", "testnet", false},
		{"https://rpc.example", "testnet", true},
		{"http://127.0.0.1:26657", "testnet", true},
		{"https://user:pass@rpc.example", "testnet", false},
		{"https://rpc.example/path", "testnet", false},
		{"https://rpc.example", "mainnet", false},
	} {
		if err := validateEndpoint(test.endpoint, test.profile); (err == nil) != test.ok {
			t.Fatalf("%+v: %v", test, err)
		}
	}
}
