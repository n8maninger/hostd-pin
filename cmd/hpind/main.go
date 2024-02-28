package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/n8maninger/hostd-pin/rate"
	"github.com/shopspring/decimal"
	"go.sia.tech/core/types"
	"go.sia.tech/hostd/api"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v3"
)

type (
	// Prices are the target prices for the host
	Prices struct {
		Storage decimal.Decimal `json:"storage"`
		Ingress decimal.Decimal `json:"ingress"`
		Egress  decimal.Decimal `json:"egress"`
	}

	// Host is a host that should have its prices updated.
	Host struct {
		Address  string `yaml:"address"`
		Password string `yaml:"password"`
	}

	// Config is the configuration for the hostd-pin application.
	Config struct {
		Hosts     []Host          `yaml:"hosts"`
		Prices    Prices          `yaml:"prices"`
		Currency  string          `yaml:"currency"`
		Frequency time.Duration   `yaml:"frequency"`
		Threshold decimal.Decimal `yaml:"threshold"`
	}
)

func isOverThreshold(a, b, percentage decimal.Decimal) bool {
	threshold := a.Mul(percentage)
	diff := a.Sub(b).Abs()
	return diff.GreaterThan(threshold)
}

func convertToCurrency(target decimal.Decimal, rate decimal.Decimal) types.Currency {
	hastings := target.Div(rate).Mul(decimal.New(1, 24)).Round(0).String()
	c, err := types.ParseCurrency(hastings)
	if err != nil {
		panic(err)
	}
	return c
}

func updateHosts(hosts []Host, target Prices, rate decimal.Decimal, log *zap.Logger) error {
	storagePrice := convertToCurrency(target.Storage, rate).Div64(4320).Div64(1e12)
	ingressPrice := convertToCurrency(target.Ingress, rate).Div64(1e12)
	egressPrice := convertToCurrency(target.Egress, rate).Div64(1e12)

	log = log.With(zap.Stringer("rate", rate), zap.Stringer("storage", storagePrice), zap.Stringer("ingress", ingressPrice), zap.Stringer("egress", egressPrice))

	for _, h := range hosts {
		client := api.NewClient(h.Address, h.Password)
		_, err := client.UpdateSettings(api.SetMinStoragePrice(storagePrice), api.SetMinIngressPrice(ingressPrice), api.SetMinEgressPrice(egressPrice))
		if err != nil {
			return fmt.Errorf("failed to update host %q: %w", h.Address, err)
		}
		log.Debug("updated host", zap.String("host", h.Address))
	}
	return nil
}

func mustLoadConfig(configPath string, cfg *Config) {
	f, err := os.Open(configPath)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)

	if err := dec.Decode(&cfg); err != nil {
		panic(err)
	}
}

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "config.yml", "path to the config file")
	flag.Parse()

	cfg := Config{
		Currency:  "usd",
		Threshold: decimal.NewFromFloat(0.1),
		Frequency: 5 * time.Minute,
		Prices: Prices{
			Storage: decimal.NewFromFloat(1.00),
			Ingress: decimal.NewFromFloat(0.10),
			Egress:  decimal.NewFromFloat(10),
		},
	}
	mustLoadConfig(configPath, &cfg)

	// configure console logging note: this is configured before anything else
	// to have consistent logging. File logging will be added after the cli
	// flags and config is parsed
	consoleCfg := zap.NewProductionEncoderConfig()
	consoleCfg.TimeKey = "" // prevent duplicate timestamps
	// consoleCfg.EncodeTime = zapcore.RFC3339TimeEncoder
	consoleCfg.EncodeDuration = zapcore.StringDurationEncoder
	consoleCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	consoleCfg.StacktraceKey = ""
	consoleCfg.CallerKey = ""
	consoleEncoder := zapcore.NewConsoleEncoder(consoleCfg)

	// only log info messages to console unless stdout logging is enabled
	consoleCore := zapcore.NewCore(consoleEncoder, zapcore.Lock(os.Stdout), zap.NewAtomicLevelAt(zap.DebugLevel))
	logger := zap.New(consoleCore, zap.AddCaller())
	defer logger.Sync()

	r := rate.New(rate.WithCurrency(cfg.Currency),
		rate.WithFrequency(cfg.Frequency),
		rate.WithLogger(logger.Named("rate")))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill, syscall.SIGTERM)
	defer cancel()

	lastRate, err := r.Update()
	if err != nil {
		log.Panic("failed to get initial exchange rate", zap.Error(err))
	}

	// set the initial rate
	if err = updateHosts(cfg.Hosts, cfg.Prices, lastRate, logger); err != nil {
		logger.Error("failed to update hosts", zap.Error(err))
	}

	go r.Run(ctx)

	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			average := r.Rate()
			if !isOverThreshold(lastRate, average, cfg.Threshold) {
				logger.Debug("skipping update", zap.Stringer("old", lastRate), zap.Stringer("new", average))
				continue
			}
			lastRate = average
			err := updateHosts(cfg.Hosts, cfg.Prices, average, logger)
			if err != nil {
				logger.Error("failed to update hosts", zap.Error(err))
			}
		case <-ctx.Done():
			return
		}
	}
}
