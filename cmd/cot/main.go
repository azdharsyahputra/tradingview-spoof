package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tvspoof "github.com/azdharsyahputra/tradingview-spoof"
)

func formatInt(n int) string {
	in := fmt.Sprintf("%d", n)
	sign := ""
	if strings.HasPrefix(in, "-") {
		sign = "-"
		in = in[1:]
	} else if n > 0 && !strings.HasPrefix(in, "+") {
		// optional sign
	}

	var out []byte
	l := len(in)
	for i, c := range in {
		if i > 0 && (l-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	return sign + string(out)
}

func formatSignedInt(n int) string {
	if n > 0 {
		return "+" + formatInt(n)
	}
	return formatInt(n)
}

func main() {
	jsonFlag := flag.Bool("json", false, "Output pure JSON")
	flag.Parse()

	args := flag.Args()
	targetSymbol := ""
	if len(args) > 0 {
		targetSymbol = args[0]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if targetSymbol != "" {
		report, err := tvspoof.FetchCOT(ctx, targetSymbol)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching COT for %s: %v\n", targetSymbol, err)
			os.Exit(1)
		}

		if *jsonFlag {
			data, _ := json.MarshalIndent(report, "", "  ")
			fmt.Println(string(data))
			return
		}

		printSingleReport(report)
		return
	}

	// Multi-asset overview
	reports, err := tvspoof.FetchAllCoreCOT(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching COT overview: %v\n", err)
		os.Exit(1)
	}

	if *jsonFlag {
		data, _ := json.MarshalIndent(reports, "", "  ")
		fmt.Println(string(data))
		return
	}

	printMultiReport(reports)
}

func printSingleReport(r *tvspoof.COTReport) {
	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════════════════════════════════════════════╗")
	fmt.Printf("║  📑  CFTC OFFICIAL COMMITMENTS OF TRADERS (COT) RADAR     %s UTC                   ║\n", time.Now().UTC().Format("2006-01-02 15:04:05"))
	fmt.Println("╠════════════════════════════════════════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  TARGET ASSET: %-15s | MARKET: %-30s | EXCHANGE: %-16s║\n", r.Symbol, truncate(r.MarketName, 30), truncate(r.Exchange, 16))
	fmt.Printf("║  REPORT DATE:  %-15s | OPEN INTEREST: %-14s | INSTITUTIONAL BIAS: %-18s║\n", r.ReportDate.Format("2006-01-02"), formatInt(r.TotalOpenInterest), truncate(r.InstitutionalRating, 18))
	fmt.Println("╠════════════════════════════════════════════════════════════════════════════════════════════════╣")
	fmt.Println("║  🏦 1. SMART MONEY / NON-COMMERCIAL POSITIONING (HEDGE FUNDS & ASSET MANAGERS)                 ║")
	fmt.Printf("║     • Non-Comm Longs  : %-14s Kontrak  (%.1f%% dari total partisipasi spekulatif)            ║\n", formatInt(r.NonCommLong), r.NonCommLongPct)
	fmt.Printf("║     • Non-Comm Shorts : %-14s Kontrak  (%.1f%% dari total partisipasi spekulatif)            ║\n", formatInt(r.NonCommShort), 100.0-r.NonCommLongPct)
	fmt.Printf("║     • Net Positioning : %-14s Kontrak  ➔ %-42s║\n", formatSignedInt(r.NonCommNet), r.InstitutionalBias)
	fmt.Println("╟────────────────────────────────────────────────────────────────────────────────────────────────╢")
	fmt.Println("║  🏭 2. COMMERCIAL HEDGERS POSITIONING (MINING, PRODUCERS & MULTINATIONALS)                     ║")
	fmt.Printf("║     • Commercial Longs: %-14s Kontrak                                                        ║\n", formatInt(r.CommLong))
	fmt.Printf("║     • Commercial Short: %-14s Kontrak (Hedging Lindung Nilai Produksi Fisik)                  ║\n", formatInt(r.CommShort))
	fmt.Printf("║     • Comm Net Pos    : %-14s Kontrak                                                        ║\n", formatSignedInt(r.CommNet))
	fmt.Println("╟────────────────────────────────────────────────────────────────────────────────────────────────╢")
	fmt.Println("║  🎯 3. STRATEGIC DESK DIRECTIVE & EXECUTION RULE                                               ║")
	fmt.Printf("║     • Status          : [%-13s]                                                            ║\n", r.InstitutionalRating)
	fmt.Printf("║     • Rule Wajib      : %-78s ║\n", truncate(r.StrategicDirective, 78))
	fmt.Println("╚════════════════════════════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()
}

func printMultiReport(reports []*tvspoof.COTReport) {
	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════════════════════════════════════════════╗")
	fmt.Printf("║  📑  CFTC OFFICIAL COMMITMENTS OF TRADERS (COT) MULTI-ASSET RADAR    %s UTC         ║\n", time.Now().UTC().Format("2006-01-02 15:04:05"))
	fmt.Println("╠════════════════════════════════════════════════════════════════════════════════════════════════╣")
	fmt.Println("║  Asset   | Market Name          | Non-Comm Long | Non-Comm Short | Net Position | Bias / Rating║")
	fmt.Println("║  ────────┼──────────────────────┼───────────────┼────────────────┼──────────────┼──────────────║")
	for _, r := range reports {
		biasIcon := "🟢"
		if r.NonCommNet < 0 {
			biasIcon = "🔴"
		} else if r.InstitutionalRating == "NEUTRAL" {
			biasIcon = "🟡"
		}
		fmt.Printf("║  %-7s | %-20s | %-13s | %-14s | %-12s | %s %-10s║\n",
			r.Symbol, truncate(r.MarketName, 20), formatInt(r.NonCommLong), formatInt(r.NonCommShort), formatSignedInt(r.NonCommNet), biasIcon, r.InstitutionalRating)
	}
	fmt.Println("╚════════════════════════════════════════════════════════════════════════════════════════════════╝")
	fmt.Println("  💡 Tip: Gunakan `go run ./cmd/cot [symbol]` untuk laporan detil per aset (contoh: `go run ./cmd/cot gold`)")
	fmt.Println()
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}
