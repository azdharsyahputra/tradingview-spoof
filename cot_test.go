package tvspoof

import (
	"context"
	"testing"
	"time"
)

func TestFetchCOTGold(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rep, err := FetchCOT(ctx, "xauusd")
	if err != nil {
		t.Fatalf("FetchCOT(xauusd) failed: %v", err)
	}

	if rep.Symbol != "XAUUSD" {
		t.Errorf("expected Symbol XAUUSD, got %s", rep.Symbol)
	}
	if rep.NonCommLong <= 0 {
		t.Errorf("expected NonCommLong > 0, got %d", rep.NonCommLong)
	}
	if rep.TotalOpenInterest <= 0 {
		t.Errorf("expected TotalOpenInterest > 0, got %d", rep.TotalOpenInterest)
	}
	t.Logf("Gold COT: %+v", rep)
}

func TestFetchCOTOil(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rep, err := FetchCOT(ctx, "usoil")
	if err != nil {
		t.Fatalf("FetchCOT(usoil) failed: %v", err)
	}
	t.Logf("Oil COT: %+v", rep)
}

func TestNormalizeCOTSymbol(t *testing.T) {
	cases := map[string]string{
		"OANDA:XAUUSD": "XAUUSD",
		"gc":           "XAUUSD",
		"TVC:USOIL":    "USOIL",
		"cl":           "USOIL",
		"FX:USDJPY":    "USDJPY",
		"6j":           "USDJPY",
		"EURUSD":       "EURUSD",
		"BTCUSD":       "BTCUSD",
	}

	for input, expected := range cases {
		got := NormalizeCOTSymbol(input)
		if got != expected {
			t.Errorf("NormalizeCOTSymbol(%q) = %q; expected %q", input, got, expected)
		}
	}
}
