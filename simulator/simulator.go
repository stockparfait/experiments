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

package simulator

import (
	"context"
	"math"

	"github.com/stockparfait/errors"
	"github.com/stockparfait/experiments"
	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/iterator"
	"github.com/stockparfait/stockparfait/db"
	"github.com/stockparfait/stockparfait/stats"
)

type Simulator struct {
	config *config.Simulator
}

var _ experiments.Experiment = &Simulator{}

func (e *Simulator) Prefix(s string) string {
	return experiments.Prefix(e.config.ID, s)
}

func (e *Simulator) AddValue(ctx context.Context, k, v string) error {
	return experiments.AddValue(ctx, e.config.ID, k, v)
}

func (e *Simulator) Run(ctx context.Context, cfg config.ExperimentConfig) error {
	var ok bool
	if e.config, ok = cfg.(*config.Simulator); !ok {
		return errors.Reason("unexpected config type: %T", cfg)
	}
	var s Strategy
	switch c := e.config.Strategy.Config.(type) {
	case *config.BuySellIntradayStrategy:
		s = &BuySellIntraday{config: c}
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
	profits := make([]float64, len(res))
	for i, r := range res {
		profits[i] = r.logProfit
	}
	if e.config.Annualize {
		for i := range profits {
			y := res[i].startDate.YearsTill(res[i].endDate)
			if y == 0 {
				profits[i] = 0
			} else {
				profits[i] /= y
			}
		}
	}
	if !e.config.LogProfit {
		for i, s := range profits {
			profits[i] = math.Exp(s)
		}
	}
	if c := e.config.ProfitPlot; c != nil {
		dist := stats.NewSampleDistribution(profits, &c.Buckets)
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
