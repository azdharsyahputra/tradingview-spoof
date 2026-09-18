package main

import (
	"testing"

	tvspoof "github.com/azdharsyahputra/tradingview-spoof"
)

func TestQuoteSnapshotMergesPartialUpdates(t *testing.T) {
	price, bid, change := 4341.2, 4341.0, 0.04
	snapshot := quoteSnapshot{Symbol: "OANDA:XAUUSD"}
	snapshot.merge(tvspoof.QuoteUpdate{Price: &price, Bid: &bid})
	snapshot.merge(tvspoof.QuoteUpdate{ChangePercent: &change})
	if snapshot.Price == nil || *snapshot.Price != price || snapshot.Bid == nil || *snapshot.Bid != bid {
		t.Fatalf("partial quote fields were not retained: %+v", snapshot)
	}
	if snapshot.ChangePercent == nil || *snapshot.ChangePercent != change {
		t.Fatalf("new field was not merged: %+v", snapshot)
	}
}
