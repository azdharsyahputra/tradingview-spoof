package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	tvspoof "github.com/azdharsyahputra/tradingview-spoof"
)

func main() {
	log.Println("Starting tradingview-spoof example...")

	// Create client with options
	client := tvspoof.NewClient(
		tvspoof.WithAutoReconnect(true),
		tvspoof.WithReconnectDelay(5*time.Second),
	)

	// Set callbacks
	client.OnQuote = func(update tvspoof.QuoteUpdate) {
		parts := fmt.Sprintf("%-20s", update.Symbol)

		if update.Price != nil {
			parts += fmt.Sprintf(" | Price: %10.2f", *update.Price)
		}
		if update.Volume != nil {
			parts += fmt.Sprintf(" | Vol: %12.2f", *update.Volume)
		}
		if update.Bid != nil && update.Ask != nil {
			parts += fmt.Sprintf(" | Spread: %.2f-%.2f", *update.Bid, *update.Ask)
		}
		if update.Change != nil && update.ChangePercent != nil {
			parts += fmt.Sprintf(" | Chg: %+.2f (%+.2f%%)", *update.Change, *update.ChangePercent)
		}

		log.Println(parts)
	}

	client.OnError = func(err error) {
		log.Printf("[ERROR] %v", err)
	}

	// Connect
	if err := client.Connect(); err != nil {
		log.Fatalf("Connect failed: %v", err)
	}

	log.Println("Connected! Subscribing to symbols...")

	// Subscribe to symbols
	client.AddSymbol("OANDA:XAUUSD")
	client.AddSymbol("BINANCE:BTCUSDT")
	client.AddSymbol("NASDAQ:AAPL")

	log.Println("Subscribed. Receiving data... (Ctrl+C to exit)")

	// Auto-exit after 30 seconds (remove this for production use)
	go func() {
		time.Sleep(30 * time.Second)
		fmt.Println("\n30s timeout reached.")
		client.Close()
		os.Exit(0)
	}()

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	client.Close()
}
