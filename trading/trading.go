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
	"fmt"

	"github.com/stockparfait/errors"
	"github.com/stockparfait/experiments"
	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/iterator"
	"github.com/stockparfait/logging"
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
	if e.config.OpenPlot != nil {
		err := experiments.PlotDistribution(ctx, stats.NewHistogramDistribution(res.open),
			e.config.OpenPlot, e.config.ID, "open")
		if err != nil {
			return errors.Annotate(err, "failed to plot open")
		}
	}
	if e.config.HighPlot != nil {
		err := experiments.PlotDistribution(ctx, stats.NewHistogramDistribution(res.high),
			e.config.HighPlot, e.config.ID, "high")
		if err != nil {
			return errors.Annotate(err, "failed to plot high")
		}
	}
	if e.config.LowPlot != nil {
		err := experiments.PlotDistribution(ctx, stats.NewHistogramDistribution(res.low),
			e.config.LowPlot, e.config.ID, "low")
		if err != nil {
			return errors.Annotate(err, "failed to plot low")
		}
	}
	if e.config.ClosePlot != nil {
		err := experiments.PlotDistribution(ctx, stats.NewHistogramDistribution(res.close),
			e.config.ClosePlot, e.config.ID, "close")
		if err != nil {
			return errors.Annotate(err, "failed to plot close")
		}
	}
	if err := e.AddValue(ctx, "tickers", fmt.Sprintf("%d", res.tickers)); err != nil {
		return errors.Annotate(err, "failed to add tickers value")
	}
	if err := e.AddValue(ctx, "samples", fmt.Sprintf("%d", res.samples)); err != nil {
		return errors.Annotate(err, "failed to add samples value")
	}
	return nil
}

type jobRes struct {
	ho      *stats.Histogram
	co      *stats.Histogram
	open    *stats.Histogram
	high    *stats.Histogram
	low     *stats.Histogram
	close   *stats.Histogram
	tickers int
	samples int
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
	j.tickers += j2.tickers
	j.samples += j2.samples
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

// logProfits computes log(t1) - log(t2) normalized by normCoeff (if !=0).
func logProfits(t1, t2 *stats.Timeseries, normCoeff float64) *stats.Timeseries {
	tss := stats.TimeseriesIntersect(t1, t2)
	t1 = tss[0]
	t2 = tss[1]
	ts := t1.Log().Sub(t2.Log())
	if normCoeff != 0 && normCoeff != 1 {
		ts = ts.DivC(normCoeff)
	}
	return ts
}

func (e *Trading) processPrices(prices []experiments.Prices) *jobRes {
	res := e.newJobRes()
	for _, p := range prices {
		open := stats.NewTimeseriesFromPrices(p.Rows, stats.PriceOpenFullyAdjusted)
		high := stats.NewTimeseriesFromPrices(p.Rows, stats.PriceHighFullyAdjusted)
		close := stats.NewTimeseriesFromPrices(p.Rows, stats.PriceCloseFullyAdjusted)
		closePrev := close.Shift(1)
		lp := close.LogProfits(1, false)
		mad := stats.NewSample(lp.Data()).MAD()
		if mad == 0 {
			logging.Warningf(e.context, "skipping %s: MAD = 0", p.Ticker)
			continue
		}
		res.tickers++
		res.samples += len(p.Rows)
		var ho *stats.Timeseries
		norm := func(c *config.DistributionPlot, n float64) float64 {
			if c.Normalize {
				return n
			}
			return 1
		}
		if e.config.HighOpenPlot != nil {
			ho = logProfits(high, open, norm(e.config.HighOpenPlot, mad))
			res.ho.Add(ho.Data()...)
		}
		if e.config.CloseOpenPlot != nil {
			if e.config.Threshold != nil && ho != nil {
				f := func(i int) bool { return ho.Data()[i] < *e.config.Threshold }
				close = close.Filter(f)
			}
			ts := logProfits(close, open, norm(e.config.CloseOpenPlot, mad))
			res.co.Add(ts.Data()...)
		}
		if e.config.OpenPlot != nil {
			ts := logProfits(open, closePrev, norm(e.config.OpenPlot, mad))
			res.open.Add(ts.Data()...)
		}
		if e.config.HighPlot != nil {
			ts := logProfits(high, closePrev, norm(e.config.HighPlot, mad))
			res.high.Add(ts.Data()...)
		}
		if e.config.LowPlot != nil {
			low := stats.NewTimeseriesFromPrices(p.Rows, stats.PriceLowFullyAdjusted)
			ts := logProfits(low, closePrev, norm(e.config.LowPlot, mad))
			res.low.Add(ts.Data()...)
		}
		if e.config.ClosePlot != nil {
			ts := logProfits(close, closePrev, norm(e.config.ClosePlot, mad))
			res.close.Add(ts.Data()...)
		}
	}
	return res
}
