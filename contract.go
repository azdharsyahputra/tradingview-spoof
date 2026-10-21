package tvspoof

import "strings"

// ContractSpec describes the retail CFD contract (Exness MT4/MT5 conventions)
// used for position sizing and PnL valuation.
type ContractSpec struct {
	Class        string  `json:"class"`         // "METAL", "CRYPTO", "ENERGY", "INDEX", "FOREX"
	ContractSize float64 `json:"contract_size"` // units per 1.0 lot
	// USDPerPointPerLot is the USD PnL of a 1.0 price move on 1.0 lot.
	USDPerPointPerLot float64 `json:"usd_per_point_per_lot"`
	Leverage          float64 `json:"leverage"`
	TypicalSpread     float64 `json:"typical_spread"` // in price units, used by the backtester
	// PipSize is the desk's pip convention in price units (gold 0.10 = "80 pips"
	// for an $8 move, oil 0.01 = cents, crypto 1.0 = points).
	PipSize float64 `json:"pip_size"`
	// Approximate is true when USD valuation could not be derived exactly
	// (e.g. non-USD crosses) and the quote currency was assumed to be USD.
	Approximate bool `json:"approximate,omitempty"`
	baseUSD     bool
}

// MarginPerLot returns the USD margin required for 1.0 lot at price.
func (s ContractSpec) MarginPerLot(price float64) float64 {
	if s.Leverage <= 0 {
		return 0
	}
	if s.baseUSD {
		// USD-base pairs (USDJPY): notional is already ContractSize USD.
		return s.ContractSize / s.Leverage
	}
	return price * s.ContractSize / s.Leverage
}

// ContractSpecFor returns the CFD contract specification for a TradingView or
// broker symbol at the given price (price is needed for USD-base forex pairs).
func ContractSpecFor(symbol string, price float64) ContractSpec {
	s := strings.ToUpper(symbol)
	if i := strings.LastIndex(s, ":"); i >= 0 {
		s = s[i+1:]
	}
	const lev = 200.0
	switch {
	case strings.Contains(s, "XAU") || strings.Contains(s, "GOLD"):
		return ContractSpec{Class: "METAL", ContractSize: 100, USDPerPointPerLot: 100, Leverage: lev, TypicalSpread: 0.20, PipSize: 0.1}
	case strings.Contains(s, "XAG") || strings.Contains(s, "SILVER"):
		return ContractSpec{Class: "METAL", ContractSize: 5000, USDPerPointPerLot: 5000, Leverage: lev, TypicalSpread: 0.025, PipSize: 0.01}
	case strings.HasPrefix(s, "BTC") || strings.HasPrefix(s, "ETH") || strings.HasPrefix(s, "SOL") || strings.HasPrefix(s, "XRP"):
		spread := 0.0
		if price > 0 {
			spread = price * 0.0002
		}
		return ContractSpec{Class: "CRYPTO", ContractSize: 1, USDPerPointPerLot: 1, Leverage: lev, TypicalSpread: spread, PipSize: 1}
	case strings.Contains(s, "OIL") || strings.Contains(s, "WTI") || strings.Contains(s, "BRENT") || s == "CL1!":
		return ContractSpec{Class: "ENERGY", ContractSize: 1000, USDPerPointPerLot: 1000, Leverage: lev, TypicalSpread: 0.03, PipSize: 0.01}
	case s == "SPX" || s == "SPX500" || s == "US500" || s == "NDX" || s == "US100" || s == "USTEC" || s == "NAS100" || s == "DJI" || s == "US30":
		return ContractSpec{Class: "INDEX", ContractSize: 1, USDPerPointPerLot: 1, Leverage: lev, TypicalSpread: 0.5, PipSize: 1}
	}
	if len(s) == 6 {
		base, quote := s[:3], s[3:]
		pip := 0.0001
		if quote == "JPY" {
			pip = 0.01
		}
		spec := ContractSpec{Class: "FOREX", ContractSize: 100000, Leverage: lev, TypicalSpread: pip * 1.2, PipSize: pip}
		switch {
		case quote == "USD":
			spec.USDPerPointPerLot = 100000
		case base == "USD" && price > 0:
			spec.USDPerPointPerLot = 100000 / price
			spec.baseUSD = true
		default:
			spec.USDPerPointPerLot = 100000
			spec.Approximate = true
		}
		return spec
	}
	return ContractSpec{Class: "UNKNOWN", ContractSize: 1, USDPerPointPerLot: 1, Leverage: lev, Approximate: true, PipSize: 1}
}

// usdExposure returns +1 when a weaker US dollar is bullish for the symbol
// (USD is the quote currency: XAUUSD, EURUSD, BTCUSD), -1 when USD is the base
// currency (USDJPY), and 0 when the dollar is not a direct pricing leg.
func usdExposure(symbol string) float64 {
	s := strings.ToUpper(symbol)
	if i := strings.LastIndex(s, ":"); i >= 0 {
		s = s[i+1:]
	}
	switch {
	case strings.HasPrefix(s, "USD") && len(s) == 6:
		return -1
	case strings.HasSuffix(s, "USD") || strings.HasSuffix(s, "USDT") || strings.Contains(s, "GOLD"):
		return 1
	}
	return 0
}

func isPreciousMetal(symbol string) bool {
	s := strings.ToUpper(symbol)
	return strings.Contains(s, "XAU") || strings.Contains(s, "GOLD") || strings.Contains(s, "XAG") || strings.Contains(s, "SILVER")
}

func isRiskAsset(symbol string) bool {
	s := strings.ToUpper(symbol)
	for _, k := range []string{"BTC", "ETH", "SOL", "SPX", "NDX", "US500", "US100", "USTEC", "NAS100", "US30", "DJI"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// Pips converts a price distance to desk pips.
func (s ContractSpec) Pips(priceDist float64) float64 {
	if s.PipSize <= 0 {
		return priceDist
	}
	return priceDist / s.PipSize
}

// USDMicro is the USD value of a price distance on 0.01 lot.
func (s ContractSpec) USDMicro(priceDist float64) float64 {
	return priceDist * s.USDPerPointPerLot * 0.01
}
