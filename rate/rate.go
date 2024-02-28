package rate

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/siacentral/apisdkgo"
	"go.uber.org/zap"
)

func getExchangeRate(currency string) (decimal.Decimal, error) {
	sc := apisdkgo.NewSiaClient()
	rates, _, err := sc.GetExchangeRate()
	if err != nil {
		return decimal.Zero, err
	}
	rate, ok := rates[currency]
	if !ok {
		return decimal.Zero, fmt.Errorf("currency not found")
	}
	return decimal.NewFromFloat(rate), nil
}

// Averager tracks the average exchange rate for a currency over a period
// of time.
type Averager struct {
	log *zap.Logger

	currency  string
	frequency time.Duration

	mu      sync.Mutex // protects rates
	rates   []decimal.Decimal
	average decimal.Decimal
}

// Update updates the average exchange rate for the configured currency.
func (ra *Averager) Update() (decimal.Decimal, error) {
	rate, err := getExchangeRate(ra.currency)
	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to get exchange rate: %w", err)
	}

	maxRates := int(48 * time.Hour / ra.frequency)

	ra.mu.Lock()
	ra.rates = append(ra.rates, rate)
	if len(ra.rates) > maxRates {
		ra.rates = ra.rates[1:]
	}
	sum := decimal.Zero
	for _, r := range ra.rates {
		sum = sum.Add(r)
	}
	ra.average = sum.Div(decimal.NewFromInt(int64(len(ra.rates))))
	ra.log.Debug("exchange rate updated", zap.Stringer("current", rate), zap.Stringer("average", ra.average))
	ra.mu.Unlock()
	return rate, nil
}

// Run starts the averager, which will update the average exchange rate
// for the configured currency at the configured frequency.
func (ra *Averager) Run(ctx context.Context) {
	ticker := time.NewTicker(ra.frequency)
	for {
		select {
		case <-ticker.C:
			_, err := ra.Update()
			if err != nil {
				ra.log.Error("failed to update exchange rate", zap.Error(err))
				continue
			}
		case <-ctx.Done():
			ticker.Stop()
			return
		}
	}
}

// Rate returns the average exchange rate for the configured currency.
func (ra *Averager) Rate() decimal.Decimal {
	ra.mu.Lock()
	defer ra.mu.Unlock()
	return ra.average
}

// New creates a new averager with the provided options.
func New(opts ...Option) *Averager {
	a := &Averager{
		log:       zap.NewNop(),
		currency:  "usd",
		frequency: 5 * time.Minute,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}
