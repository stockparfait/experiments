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

// Package trading is an experiment in exploiting volatility for profit.
package trading

import (
	"context"

	"github.com/stockparfait/errors"
	"github.com/stockparfait/experiments"
	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/iterator"
	"github.com/stockparfait/logging"
	"github.com/stockparfait/stockparfait/db"
	"github.com/stockparfait/stockparfait/stats"
)

type Trading struct {
	config  *config.Trading
	context context.Context
}

var _ experiments.Experiment = &Trading{}

func (e *Trading) Prefix(s string) string {
	return experiments.Prefix(e.config.ID, s)
}

func (e *Trading) AddValue(ctx context.Context, k, v string) error {
	return experiments.AddValue(ctx, e.config.ID, k, v)
}

func (e *Trading) Run(ctx context.Context, cfg config.ExperimentConfig) error {
	e.context = ctx
	var ok bool
	if e.config, ok = cfg.(*config.Trading); !ok {
		return errors.Reason("unexpected config type: %T", cfg)
	}
	if err := e.processData(ctx); err != nil {
		return errors.Annotate(err, "failled to process prcie data")
	}
	return nil
}

func (e *Trading) processData(ctx context.Context) error {
	it, err := experiments.SourceMapPrices(ctx, e.config.Data, e.processPrices)
	if err != nil {
		return errors.Annotate(err, "failed to process data")
	}
	defer it.Close()
	f := func(res, j *jobRes) *jobRes { return res.Merge(j) }
	res := iterator.Reduce[*jobRes](it, e.newJobRes(), f)
	if e.config.HighOpenPlot != nil {
		err := experiments.PlotDistribution(ctx, stats.NewHistogramDistribution(res.ho),
			e.config.HighOpenPlot, e.config.ID, "high/open")
		if err != nil {
			return errors.Annotate(err, "failed to plot high/open")
		}
	}
	if e.config.CloseOpenPlot != nil {
		err := experiments.PlotDistribution(ctx, stats.NewHistogramDistribution(res.co),
			e.config.CloseOpenPlot, e.config.ID, "close/open")
		if err != nil {
			return errors.Annotate(err, "failed to plot close/open")
		}
	}
	// TODO: finish the other plots.
	return nil
}

type jobRes struct {
	ho    *stats.Histogram
	co    *stats.Histogram
	open  *stats.Histogram
	high  *stats.Histogram
	low   *stats.Histogram
	close *stats.Histogram
}

// Merge j2 into j and return it.
func (j *jobRes) Merge(j2 *jobRes) *jobRes {
	if j.ho != nil && j2.ho != nil {
		if err := j.ho.AddHistogram(j2.ho); err != nil {
			panic(errors.Annotate(err, "failed to merge high/open histogram"))
		}
	}
	if j.co != nil && j2.co != nil {
		if err := j.co.AddHistogram(j2.co); err != nil {
			panic(errors.Annotate(err, "failed to merge close/open histogram"))
		}
	}
	if j.open != nil && j2.open != nil {
		if err := j.open.AddHistogram(j2.open); err != nil {
			panic(errors.Annotate(err, "failed to merge open histogram"))
		}
	}
	if j.high != nil && j2.high != nil {
		if err := j.high.AddHistogram(j2.high); err != nil {
			panic(errors.Annotate(err, "failed to merge high histogram"))
		}
	}
	if j.low != nil && j2.low != nil {
		if err := j.low.AddHistogram(j2.low); err != nil {
			panic(errors.Annotate(err, "failed to merge low histogram"))
		}
	}
	if j.close != nil && j2.close != nil {
		if err := j.close.AddHistogram(j2.close); err != nil {
			panic(errors.Annotate(err, "failed to merge close histogram"))
		}
	}
	return j
}

func (e *Trading) newJobRes() *jobRes {
	var r jobRes
	if e.config.HighOpenPlot != nil {
		r.ho = stats.NewHistogram(&e.config.HighOpenPlot.Buckets)
	}
	if e.config.CloseOpenPlot != nil {
		r.co = stats.NewHistogram(&e.config.CloseOpenPlot.Buckets)
	}
	if e.config.OpenPlot != nil {
		r.open = stats.NewHistogram(&e.config.OpenPlot.Buckets)
	}
	if e.config.HighPlot != nil {
		r.high = stats.NewHistogram(&e.config.HighPlot.Buckets)
	}
	if e.config.LowPlot != nil {
		r.low = stats.NewHistogram(&e.config.LowPlot.Buckets)
	}
	if e.config.ClosePlot != nil {
		r.close = stats.NewHistogram(&e.config.ClosePlot.Buckets)
	}
	return &r
}

func addLogProfits(t1, t2 *stats.Timeseries, normCoeff float64, c *config.DistributionPlot, h *stats.Histogram, ticker string) (*stats.Timeseries, error) {
	if c == nil {
		return nil, nil
	}
	tss := stats.TimeseriesIntersect(t1, t2)
	t1 = tss[0]
	t2 = tss[1]
	ts := t1.Log().Sub(t2.Log())
	if c.Normalize {
		if normCoeff == 0 {
			return nil, errors.Reason("cannot normalize %s", ticker)
		}
		ts = ts.DivC(normCoeff)
	}
	h.Add(ts.Data()...)
	return ts, nil
}

// Filter Timeseries points using the filter function f.
func filterTS(ts *stats.Timeseries, f func(i int) bool) *stats.Timeseries {
	var dates []db.Date
	var data []float64
	for i, d := range ts.Data() {
		if f(i) {
			dates = append(dates, ts.Dates()[i])
			data = append(data, d)
		}
	}
	return stats.NewTimeseries(dates, data)
}

func (e *Trading) processPrices(prices []experiments.Prices) *jobRes {
	res := e.newJobRes()
	for _, p := range prices {
		open := stats.NewTimeseriesFromPrices(p.Rows, stats.PriceOpenFullyAdjusted)
		high := stats.NewTimeseriesFromPrices(p.Rows, stats.PriceHighFullyAdjusted)
		close := stats.NewTimeseriesFromPrices(p.Rows, stats.PriceCloseFullyAdjusted)
		// closePrev := close.Shift(1)
		lp := close.LogProfits(1)
		mad := stats.NewSample(lp.Data()).MAD()
		var ho *stats.Timeseries
		var err error
		if e.config.HighOpenPlot != nil {
			ho, err = addLogProfits(high, open, mad, e.config.HighOpenPlot, res.ho, p.Ticker)
			if err != nil {
				logging.Warningf(e.context, "skipping %s:\n %s", p.Ticker, err.Error())
				continue
			}
		}
		if e.config.CloseOpenPlot != nil {
			if e.config.Threshold != nil && ho != nil {
				f := func(i int) bool { return ho.Data()[i] < *e.config.Threshold }
				close = filterTS(close, f)
			}
			_, err := addLogProfits(close, open, mad, e.config.CloseOpenPlot, res.co, p.Ticker)
			if err != nil {
				logging.Warningf(e.context, "skipping %s:\n %s", p.Ticker, err.Error())
				continue
			}
		}
		// TODO: add the other plots.
	}
	return res
}
