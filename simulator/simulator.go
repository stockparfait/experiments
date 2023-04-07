// Copyright 2023 Stock Parfait

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

//     http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package simulator experiments with simulating various strategies.
package simulator

import (
	"context"
	"math"

	"github.com/stockparfait/errors"
	"github.com/stockparfait/experiments"
	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/iterator"
	"github.com/stockparfait/logging"
	"github.com/stockparfait/stockparfait/db"
	"github.com/stockparfait/stockparfait/stats"
)

type Simulator struct {
	config  *config.Simulator
	context context.Context
}

var _ experiments.Experiment = &Simulator{}

func (e *Simulator) Prefix(s string) string {
	return experiments.Prefix(e.config.ID, s)
}

func (e *Simulator) AddValue(ctx context.Context, k, v string) error {
	return experiments.AddValue(ctx, e.config.ID, k, v)
}

func (e *Simulator) Run(ctx context.Context, cfg config.ExperimentConfig) error {
	e.context = ctx
	var ok bool
	if e.config, ok = cfg.(*config.Simulator); !ok {
		return errors.Reason("unexpected config type: %T", cfg)
	}
	var s Strategy
	switch c := e.config.Strategy.Config.(type) {
	case *config.BuySellIntradayStrategy:
		s = &BuySellIntraday{
			config: c,
		}
	default:
		return errors.Reason(`unsupported strategy "%s"`, c.Name())
	}
	res, err := e.executeStrategy(ctx, s)
	if err != nil {
		return errors.Annotate(err, "failled to execute strategy")
	}
	if err := e.reportResults(ctx, res); err != nil {
		return errors.Annotate(err, "failed to report results")
	}
	return nil
}

// transaction - buy or sell within a strategy run.
type transaction struct {
	buy    bool // buy or sell type
	date   db.Date
	amount float64 // portion of the total value, in [0..1]
}

// strategyResult for a single ticker run of a strategy.
type strategyResult struct {
	logProfit    float64
	startDate    db.Date
	endDate      db.Date
	transactions []transaction // optional
}

func (s strategyResult) IsZero() bool { return s.startDate.IsZero() }

func (e *Simulator) reportResults(ctx context.Context, res []strategyResult) error {
	samples := make([]float64, len(res))
	for i, r := range res {
		samples[i] = r.logProfit
	}
	if e.config.Annualize {
		for i := range samples {
			y := res[i].startDate.YearsTill(res[i].endDate)
			if y == 0 {
				samples[i] = 0
			} else {
				samples[i] /= y
			}
		}
	}
	if !e.config.LogProfit {
		for i, s := range samples {
			samples[i] = math.Exp(s)
		}
	}
	if c := e.config.ProfitPlot; c != nil {
		dist := stats.NewSampleDistribution(samples, &c.Buckets)
		err := experiments.PlotDistribution(ctx, dist, c, e.config.ID, "log-profits")
		if err != nil {
			return errors.Annotate(err, "failed to plot profits")
		}
	}
	return nil
}

// Strategy API.
type Strategy interface {
	// Concurrency-safe strategy execution for a single ticker. A zero result
	// means the strategy didn't apply, no transactions were executed. When
	// "xactions" is true, the list of transactions is generated in the result.
	ExecuteTicker(ctx context.Context, lp experiments.LogProfits, xactions bool) strategyResult
}

func (e *Simulator) executeStrategy(ctx context.Context, s Strategy) ([]strategyResult, error) {
	f := func(lps []experiments.LogProfits) []strategyResult {
		var res []strategyResult
		for _, lp := range lps {
			r := s.ExecuteTicker(ctx, lp, false)
			if !r.IsZero() {
				res = append(res, r)
			}
		}
		return res
	}
	it, err := experiments.SourceMap(ctx, e.config.Data, f)
	if err != nil {
		return nil, errors.Annotate(err,
			`failed to execute "%s"`, e.config.Strategy.Name())
	}
	defer it.Close()
	rf := func(res, r []strategyResult) []strategyResult { return append(res, r...) }
	res := iterator.Reduce[[]strategyResult](it, nil, rf)
	return res, nil
}

// BuySellIntraday is a configurable day trading strategy.
//
// TODO: add open & close time, and trigger buy / sell at the event, not
// sometimes after. Do not try to guess the open / close moments. In particular,
// this allows for one transaction per bar, because right now we have to detect
// close at the open time and may add two transactions, one for each at the same
// time.
type BuySellIntraday struct {
	config *config.BuySellIntradayStrategy
}

var _ Strategy = &BuySellIntraday{}

func (s BuySellIntraday) ExecuteTicker(ctx context.Context, lp experiments.LogProfits, xactions bool) strategyResult {
	var res strategyResult
	if len(lp.Timeseries.Data()) == 0 {
		logging.Warningf(ctx, "skipping %s: not enough price data", lp.Ticker)
		return res
	}
	var bought bool
	var tradedToday bool
	// Cumulative log-profit and the max. observed log-profit for the current
	// position, and the log-profit for the entire strategy.
	var logProfit, totalLogProfit float64
	maxLogProfit := math.Inf(-1)
	var startDay, currDay db.Date
	for i, p := range lp.Timeseries.Data() {
		date := lp.Timeseries.Dates()[i]
		day := date.Date()
		if i == 0 {
			startDay = day
		}
		if day != currDay {
			tradedToday = false
		}
		currDay = day
		if bought {
			logProfit += p
			if s.sell(date, logProfit, maxLogProfit) {
				bought = false
				tradedToday = true
				totalLogProfit += logProfit
				logProfit = 0
				maxLogProfit = 0
				if xactions {
					res.transactions = append(res.transactions, transaction{
						buy: false, date: date, amount: 1})
				}
				continue
			}
			if logProfit > maxLogProfit {
				maxLogProfit = logProfit
			}
			continue
		}
		if s.buy(date, tradedToday) {
			logProfit = 0
			maxLogProfit = 0
			bought = true
			tradedToday = true
			if xactions {
				res.transactions = append(res.transactions, transaction{
					buy: true, date: date, amount: 1})
			}
		}
	}
	if bought {
		totalLogProfit += logProfit
	}
	res.logProfit = totalLogProfit
	res.startDate = startDay
	res.endDate = currDay
	return res
}

func (s BuySellIntraday) buy(date db.Date, tradedToday bool) bool {
	return !tradedToday && s.config.Buy <= date.Time
}

// sell checks if a sell condition is met and computes the resulting log-profit
// from the cost basis. It takes the current and previous day of the current
// bar, the bar's log-profit, the remaining cumulative log-profit since buy, and
// the maximum observed cumulative log-profit since buy.
func (s BuySellIntraday) sell(date db.Date, logProfit, maxLogProfit float64) bool {
	for _, c := range s.config.Sell {
		switch {
		case c.Time != nil:
			if *c.Time <= date.Time {
				return true
			}
		case c.Target > 1:
			if logProfit >= math.Log(c.Target) { // TODO: cache the log
				return true
			}
		case c.StopLoss > 0:
			if logProfit <= math.Log(c.StopLoss) { // TODO: cache the log
				return true
			}
		case c.StopLossTrailing > 0:
			if logProfit <= maxLogProfit+math.Log(c.StopLossTrailing) { // TODO: cache the log
				return true
			}
		}
	}
	return false
}
