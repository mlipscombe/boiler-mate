package webui

import (
	"slices"
	"strings"
	"time"
)

const kgPerPound = 0.45359237

type pelletPurchase struct {
	ID       string    `json:"id"`
	At       time.Time `json:"at"`
	Quantity float64   `json:"quantity"`
	Weight   float64   `json:"weight"`
	Unit     string    `json:"unit"`
	Brand    string    `json:"brand"`
	Price    *float64  `json:"price"`
}

type pelletSettings struct {
	Currency  string  `json:"currency"`
	LowWeight float64 `json:"lowWeight"`
	LowUnit   string  `json:"lowUnit"`
}

type pelletPeriod struct {
	Kind     string    `json:"kind"`
	From     time.Time `json:"from"`
	To       time.Time `json:"to"`
	UsedKG   float64   `json:"usedKg"`
	PricedKG float64   `json:"pricedKg"`
	Cost     float64   `json:"cost"`
}

type pelletReport struct {
	Settings    pelletSettings   `json:"settings"`
	Purchases   []pelletPurchase `json:"purchases"`
	Periods     []pelletPeriod   `json:"periods"`
	Brands      []pelletBrand    `json:"brands"`
	TrackedFrom *time.Time       `json:"trackedFrom"`
	SampledAt   *time.Time       `json:"sampledAt"`
	RemainingKG *float64         `json:"remainingKg"`
}

type pelletBrand struct {
	Name        string  `json:"name"`
	PurchasedKG float64 `json:"purchasedKg"`
	UsedKG      float64 `json:"usedKg"`
	PricedKG    float64 `json:"pricedKg"`
	Cost        float64 `json:"cost"`
	FirstUsed   int64   `json:"firstUsed,omitempty"`
	LastUsed    int64   `json:"lastUsed,omitempty"`
}

func pelletWeightKG(weight float64, unit string) float64 {
	if unit == "lb" {
		return weight * kgPerPound
	}
	return weight
}

func pelletPeriods(now time.Time, history string) []pelletPeriod {
	hour := now.Add(-time.Duration(now.Minute()*60+now.Second())*time.Second - time.Duration(now.Nanosecond()))
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	week := day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
	month := day.AddDate(0, 0, 1-day.Day())
	year := month.AddDate(0, 1-int(month.Month()), 0)
	current := []pelletPeriod{
		{Kind: "hour", From: hour, To: now}, {Kind: "day", From: day, To: now},
		{Kind: "week", From: week, To: now}, {Kind: "month", From: month, To: now},
		{Kind: "year", From: year, To: now},
	}
	if history == "" {
		return current
	}
	counts := []int{24, 31, 12, 12, 5}
	for index, period := range current {
		if period.Kind != history {
			continue
		}
		periods := make([]pelletPeriod, 0, counts[index])
		for i := 0; i < counts[index]; i++ {
			periods = append(periods, period)
			period.To = period.From
			switch history {
			case "hour":
				period.From = period.From.Add(-time.Hour)
			case "day":
				period.From = period.From.AddDate(0, 0, -1)
			case "week":
				period.From = period.From.AddDate(0, 0, -7)
			case "month":
				period.From = period.From.AddDate(0, -1, 0)
			case "year":
				period.From = period.From.AddDate(-1, 0, 0)
			}
		}
		return periods
	}
	return nil
}

type pelletLot struct {
	remaining float64
	priceKG   *float64
	brand     *pelletBrand
}

// Purchases and use arrive in chronological order. Split each counter delta at
// purchase and reporting boundaries, estimating even use between counter reads.
type pelletAllocation struct {
	pending    []pelletPurchase
	lots       []pelletLot
	periods    []pelletPeriod
	boundaries []int64
	brands     map[string]*pelletBrand
}

func allocatePellets(purchases []pelletPurchase, periods []pelletPeriod) pelletAllocation {
	var boundaries []int64
	for _, period := range periods {
		boundaries = append(boundaries, period.From.Unix(), period.To.Unix())
	}
	slices.Sort(boundaries)
	return pelletAllocation{pending: purchases, periods: periods, boundaries: slices.Compact(boundaries),
		brands: make(map[string]*pelletBrand)}
}

func (a *pelletAllocation) addPurchases(at int64) {
	for len(a.pending) > 0 && a.pending[0].At.Unix() <= at {
		purchase := a.pending[0]
		a.pending = a.pending[1:]
		basisKG := pelletWeightKG(purchase.Weight, purchase.Unit)
		key := strings.ToLower(purchase.Brand)
		brand := a.brands[key]
		if brand == nil {
			brand = &pelletBrand{Name: purchase.Brand}
			a.brands[key] = brand
		}
		lot := pelletLot{remaining: purchase.Quantity * basisKG, brand: brand}
		brand.PurchasedKG += lot.remaining
		if purchase.Price != nil {
			priceKG := *purchase.Price / basisKG
			lot.priceKG = &priceKG
		}
		a.lots = append(a.lots, lot)
	}
}

func (a *pelletAllocation) use(from, to int64, kg float64) {
	seconds := float64(to - from)
	for start := from; start < to; {
		a.addPurchases(start)
		for len(a.boundaries) > 0 && a.boundaries[0] <= start {
			a.boundaries = a.boundaries[1:]
		}
		end := to
		if len(a.pending) > 0 {
			end = min(end, a.pending[0].At.Unix())
		}
		if len(a.boundaries) > 0 {
			end = min(end, a.boundaries[0])
		}
		used := kg * float64(end-start) / seconds
		priced, cost := a.consume(used, start, end)
		for i := range a.periods {
			period := &a.periods[i]
			if start >= period.From.Unix() && start < period.To.Unix() {
				period.UsedKG += used
				period.PricedKG += priced
				period.Cost += cost
			}
		}
		start = end
	}
}

func (a *pelletAllocation) consume(kg float64, from, to int64) (priced, cost float64) {
	for kg > 0 && len(a.lots) > 0 {
		lot := &a.lots[0]
		used := min(lot.remaining, kg)
		if lot.brand.FirstUsed == 0 {
			lot.brand.FirstUsed = from
		}
		lot.brand.LastUsed = to
		lot.brand.UsedKG += used
		if lot.priceKG != nil {
			priced += used
			cost += used * *lot.priceKG
			lot.brand.PricedKG += used
			lot.brand.Cost += used * *lot.priceKG
		}
		lot.remaining -= used
		kg -= used
		if lot.remaining == 0 {
			a.lots = a.lots[1:]
		}
	}
	return priced, cost
}

func (a *pelletAllocation) brandSummary() []pelletBrand {
	brands := make([]pelletBrand, 0, len(a.brands))
	for _, brand := range a.brands {
		brands = append(brands, *brand)
	}
	slices.SortFunc(brands, func(a, b pelletBrand) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return brands
}

func (a *pelletAllocation) remaining(at int64) float64 {
	a.addPurchases(at)
	var kg float64
	for _, lot := range a.lots {
		kg += lot.remaining
	}
	return kg
}
