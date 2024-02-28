package rate

import (
	"time"

	"go.uber.org/zap"
)

// An Option configures an averager.
type Option func(*Averager)

// WithCurrency sets the currency for the averager.
func WithCurrency(currency string) Option {
	return func(ra *Averager) {
		ra.currency = currency
	}
}

// WithFrequency sets the frequency for the averager.
func WithFrequency(frequency time.Duration) Option {
	return func(ra *Averager) {
		ra.frequency = frequency
	}
}

// WithLogger sets the logger for the averager.
func WithLogger(log *zap.Logger) Option {
	return func(ra *Averager) {
		ra.log = log
	}
}
