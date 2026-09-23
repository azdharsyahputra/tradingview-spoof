package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tvspoof "github.com/azdharsyahputra/tradingview-spoof"
)

const (
	cReset   = "\033[0m"
	cBold    = "\033[1m"
	cRed     = "\033[31m"
	cGreen   = "\033[32m"
	cYellow  = "\033[33m"
	cCyan    = "\033[36m"
	cMagenta = "\033[35m"
	cGray    = "\033[90m"
	cWhite   = "\033[97m"
)

func printBanner() {
	fmt.Println(cBold + cCyan + "╔═══════════════════════════════════════════════════════════════════════════════╗" + cReset)
	fmt.Println(cBold + cCyan + "║  ⚡ ZTERM EXNESS EXECUTION ENGINE  •  PROP TRADING ORDER TERMINAL             ║" + cReset)
	fmt.Println(cBold + cCyan + "╚═══════════════════════════════════════════════════════════════════════════════╝" + cReset)
}

func printHelp() {
	printBanner()
	cfg, err := tvspoof.LoadExnessConfig()
	status := cGreen + "CONFIGURED" + cReset
	if err != nil {
		status = cRed + "NOT CONFIGURED (Missing EXNESS_TOKEN / EXNESS_ACCOUNT_ID in .env)" + cReset
	} else {
		status = fmt.Sprintf("%s (Server: %s | Account: %s)", cGreen+"CONNECTED", cfg.Server, cfg.AccountID+cReset)
	}

	fmt.Printf("Status: %s\n\n", status)
	fmt.Println(cBold + "Usage:" + cReset)
	fmt.Println("  go run ./cmd/order " + cYellow + "positions" + cReset + "                 Lihat semua open posisi & PnL")
	fmt.Println("  go run ./cmd/order " + cGreen + "buy" + cReset + " <symbol> <lot> [sl] [tp]   Buka posisi BUY (market execution)")
	fmt.Println("  go run ./cmd/order " + cRed + "sell" + cReset + " <symbol> <lot> [sl] [tp]  Buka posisi SELL (market execution)")
	fmt.Println("  go run ./cmd/order " + cYellow + "close" + cReset + " <position_id> [lot]    Tutup posisi berdasarkan ID")
	fmt.Println("  go run ./cmd/order " + cRed + "close all" + cReset + "                     Tutup SEMUA posisi terbuka sekaligus")
	fmt.Println("  go run ./cmd/order " + cCyan + "config" + cReset + " <token> <acc_id> [srv]  Set/Update credentials ke .env")
	fmt.Println()
	fmt.Println(cBold + "Contoh Eksekusi:" + cReset)
	fmt.Println("  go run ./cmd/order buy xauusd 0.01 4345.50 4368.50")
	fmt.Println("  go run ./cmd/order sell xauusd 0.02 4358.00 4334.30")
	fmt.Println("  go run ./cmd/order close 12345678")
	fmt.Println("  go run ./cmd/order close all")
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	action := strings.ToLower(os.Args[1])
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	switch action {
	case "config":
		handleConfig()
	case "positions", "pos", "list":
		handlePositions(ctx)
	case "buy":
		handleOrder(ctx, "BUY")
	case "sell":
		handleOrder(ctx, "SELL")
	case "close":
		handleClose(ctx)
	default:
		printHelp()
	}
}

func handleConfig() {
	if len(os.Args) < 4 {
		fmt.Println(cRed + "❌ Format salah: go run ./cmd/order config <token> <account_id> [server]" + cReset)
		return
	}
	token := os.Args[2]
	accountID := os.Args[3]
	server := "trial6"
	if len(os.Args) >= 5 {
		server = os.Args[4]
	}

	if err := tvspoof.SaveExnessCredentials(token, accountID, server, ""); err != nil {
		fmt.Printf(cRed+"❌ Gagal menyimpan konfigurasi: %v\n"+cReset, err)
		return
	}
	fmt.Println(cGreen + "✅ Konfigurasi Exness berhasil disimpan ke .env!" + cReset)
	fmt.Printf("   Account ID: %s | Server: %s\n", accountID, server)
}

func handlePositions(ctx context.Context) {
	printBanner()
	cfg, err := tvspoof.LoadExnessConfig()
	if err != nil {
		fmt.Printf(cRed+"❌ Konfigurasi belum disetel: %v\n"+cReset, err)
		fmt.Println("Gunakan: go run ./cmd/order config <token> <account_id> [server]")
		return
	}

	fmt.Printf("🔍 Mengambil open positions untuk akun %s%s%s (Server: %s)...\n\n",
		cBold+cYellow, cfg.AccountID, cReset, cfg.Server)

	positions, err := tvspoof.GetExnessPositions(ctx)
	if err != nil {
		fmt.Printf(cRed+"❌ Gagal mengambil open positions: %v\n"+cReset, err)
		return
	}

	if len(positions) == 0 {
		fmt.Println(cYellow + "ℹ️ Tidak ada posisi terbuka saat ini." + cReset)
		return
	}

	fmt.Printf(cBold+"%-12s | %-10s | %-6s | %-6s | %-10s | %-10s | %-10s | %-10s\n"+cReset,
		"ID", "SYMBOL", "TYPE", "LOT", "OPEN", "SL", "TP", "PROFIT ($)")
	fmt.Println(strings.Repeat("─", 88))

	totalProfit := 0.0
	for _, p := range positions {
		typeColor := cGreen
		if p.Type == "SELL" {
			typeColor = cRed
		}
		profitColor := cGreen
		if p.Profit < 0 {
			profitColor = cRed
		}

		slStr := fmt.Sprintf("%.2f", p.SL)
		if p.SL <= 0 {
			slStr = "-"
		}
		tpStr := fmt.Sprintf("%.2f", p.TP)
		if p.TP <= 0 {
			tpStr = "-"
		}

		fmt.Printf("%-12s | %-10s | %s%-6s%s | %-6.2f | %-10.2f | %-10s | %-10s | %s%+10.2f%s\n",
			p.ID, p.Instrument, typeColor, p.Type, cReset, p.Volume, p.OpenPrice, slStr, tpStr, profitColor, p.Profit, cReset)
		totalProfit += p.Profit
	}
	fmt.Println(strings.Repeat("─", 88))

	totColor := cGreen
	if totalProfit < 0 {
		totColor = cRed
	}
	fmt.Printf("Total Floating PnL: %s%s$%.2f%s (Total: %d posisi)\n", cBold, totColor, totalProfit, cReset, len(positions))
}

func handleOrder(ctx context.Context, direction string) {
	printBanner()
	if len(os.Args) < 4 {
		fmt.Printf(cRed+"❌ Format salah: go run ./cmd/order %s <symbol> <lot> [sl] [tp]\n"+cReset, strings.ToLower(direction))
		fmt.Printf("Contoh: go run ./cmd/order %s xauusd 0.01 4358.00 4334.30\n", strings.ToLower(direction))
		return
	}

	symbol := os.Args[2]
	vol, err := strconv.ParseFloat(os.Args[3], 64)
	if err != nil || vol <= 0 {
		fmt.Println(cRed + "❌ Nilai lot tidak valid" + cReset)
		return
	}

	sl := 0.0
	tp := 0.0
	if len(os.Args) >= 5 {
		sl, _ = strconv.ParseFloat(os.Args[4], 64)
	}
	if len(os.Args) >= 6 {
		tp, _ = strconv.ParseFloat(os.Args[5], 64)
	}

	inst := tvspoof.NormalizeExnessSymbol(symbol)
	dirColor := cGreen
	if direction == "SELL" {
		dirColor = cRed
	}

	fmt.Printf("🚀 Mempersiapkan order %s%s%s %.2f lot %s%s%s...\n",
		cBold+dirColor, direction, cReset, vol, cBold+cYellow, inst, cReset)
	if sl > 0 || tp > 0 {
		fmt.Printf("   SL: %.2f | TP: %.2f\n", sl, tp)
	}
	fmt.Println("📡 Menghubungkan ke live tick feed Exness...")

	res, err := tvspoof.ExecuteExnessOrder(ctx, tvspoof.ExnessOrderParams{
		Symbol:    inst,
		Direction: direction,
		Volume:    vol,
		SL:        sl,
		TP:        tp,
	})
	if err != nil {
		fmt.Printf(cRed+"❌ Order GAGAL: %v\n"+cReset, err)
		return
	}

	fmt.Printf(cBold+cGreen+"✅ Order %s BERHASIL dieksekusi!\n"+cReset, direction)
	fmt.Printf("Response: %+v\n\n", res)

	// Fetch updated positions
	time.Sleep(1 * time.Second)
	handlePositions(ctx)
}

func handleClose(ctx context.Context) {
	printBanner()
	if len(os.Args) < 3 {
		fmt.Println(cRed + "❌ Format salah: go run ./cmd/order close <position_id | all> [lot]" + cReset)
		return
	}

	target := strings.ToLower(os.Args[2])
	if target == "all" {
		fmt.Println(cBold + cYellow + "⚠️ Menutup SEMUA posisi terbuka..." + cReset)
		closed, err := tvspoof.CloseAllExnessPositions(ctx)
		if err != nil {
			fmt.Printf(cRed+"❌ Gagal menutup beberapa posisi: %v\n"+cReset, err)
		}
		fmt.Printf(cBold+cGreen+"✅ Selesai! %d posisi berhasil ditutup: %v\n"+cReset, len(closed), closed)
		return
	}

	vol := 0.0
	if len(os.Args) >= 4 {
		vol, _ = strconv.ParseFloat(os.Args[3], 64)
	}

	fmt.Printf("⚡ Menutup posisi #%s...\n", target)
	res, err := tvspoof.CloseExnessPosition(ctx, target, vol)
	if err != nil {
		fmt.Printf(cRed+"❌ Gagal menutup posisi #%s: %v\n"+cReset, target, err)
		return
	}

	fmt.Printf(cBold+cGreen+"✅ Posisi #%s berhasil ditutup!\n"+cReset, target)
	fmt.Printf("Response: %+v\n", res)
}
