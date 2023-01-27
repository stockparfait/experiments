// Copyright 2022 Stock Parfait

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

//     http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package distribution is an experiment plotting distributions of log-profits.
package distribution

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

// Distribution is an Experiment implementation for displaying and researching
// distributions of log-profits.
type Distribution struct {
	config         *config.Distribution
	context        context.Context
	histogram      *stats.Histogram
	meansHistogram *stats.Histogram
	madsHistogram  *stats.Histogram
	numTickers     int
	tickers        []string
}

var _ experiments.Experiment = &Distribution{}
var _ iterator.Iterator[[]string] = &Distribution{}

func (d *Distribution) Prefix(s string) string {
	return experiments.Prefix(d.config.ID, s)
}

func (d *Distribution) AddValue(ctx context.Context, k, v string) error {
	return experiments.AddValue(ctx, d.config.ID, k, v)
}

func (d *Distribution) Run(ctx context.Context, cfg config.ExperimentConfig) error {
	var ok bool
	if d.config, ok = cfg.(*config.Distribution); !ok {
		return errors.Reason("unexpected config type: %T", cfg)
	}
	d.context = ctx
	if d.config.LogProfits != nil {
		d.histogram = stats.NewHistogram(&d.config.LogProfits.Buckets)
	}
	tickers, err := d.config.Reader.Tickers(ctx)
	if err != nil {
		return errors.Annotate(err, "failed to list tickers")
	}
	if err := d.processTickers(tickers); err != nil {
		return errors.Annotate(err, "failed to process tickers")
	}
	if err := d.AddValue(ctx, "tickers", fmt.Sprintf("%d", d.numTickers)); err != nil {
		return errors.Annotate(err, "failed to add '%s' tickers value", d.config.ID)
	}
	if d.histogram != nil {
		if err := d.AddValue(ctx, "samples", fmt.Sprintf("%d", d.histogram.CountsTotal())); err != nil {
			return errors.Annotate(err, "failed to add '%s' samples value", d.config.ID)
		}
	}
	if d.histogram.CountsTotal() == 0 {
		return nil
	}
	if err := experiments.PlotDistribution(ctx, stats.NewHistogramDistribution(d.histogram), d.config.LogProfits, d.config.ID, "log-profit"); err != nil {
		return errors.Annotate(err, "failed to plot '%s' sample distribution", d.config.ID)
	}
	if err := experiments.PlotDistribution(ctx, stats.NewHistogramDistribution(d.meansHistogram), d.config.Means, d.config.ID, "means"); err != nil {
		return errors.Annotate(err, "failed to plot '%s' means distribution", d.config.ID)
	}
	if err := experiments.PlotDistribution(ctx, stats.NewHistogramDistribution(d.madsHistogram), d.config.MADs, d.config.ID, "MADs"); err != nil {
		return errors.Annotate(err, "failed to plot '%s' MADs distribution", d.config.ID)
	}
	if d.meansHistogram != nil {
		if err := d.AddValue(ctx, "average mean", fmt.Sprintf("%.4g", d.meansHistogram.Mean())); err != nil {
			return errors.Annotate(err,
				"failed to add '%s' average mean value", d.config.ID)
		}
	}
	if d.madsHistogram != nil {
		if err := d.AddValue(ctx, "average MAD", fmt.Sprintf("%.4g", d.madsHistogram.Mean())); err != nil {
			return errors.Annotate(err,
				"failed to add '%s' average MAD value", d.config.ID)
		}
	}
	return nil
}

type jobResult struct {
	Histogram  *stats.Histogram
	Means      []float64
	MADs       []float64
	NumTickers int
	Err        error
}

func reduceJobResult(j, j2 *jobResult) *jobResult {
	if j.Err != nil {
		return j
	}
	if j2.Err != nil {
		j.Err = errors.Annotate(j2.Err, "job failed")
		return j
	}
	if j.Histogram != nil {
		j.Histogram.AddHistogram(j2.Histogram)
	}
	j.Means = append(j.Means, j2.Means...)
	j.MADs = append(j.MADs, j2.MADs...)
	j.NumTickers += j2.NumTickers
	return j
}

func (d *Distribution) newJobResult() *jobResult {
	res := &jobResult{}
	if d.histogram != nil {
		res.Histogram = stats.NewHistogram(d.histogram.Buckets())
	}
	return res
}

func (d *Distribution) processTicker(ticker string, res *jobResult) error {
	rows, err := d.config.Reader.Prices(ticker)
	if err != nil {
		logging.Warningf(d.context, err.Error())
		return nil
	}
	if len(rows) <= 1 {
		return nil
	}
	ts := stats.NewTimeseries().FromPrices(rows, stats.PriceFullyAdjusted)
	sample := ts.LogProfits(d.config.Compound)
	res.Means = append(res.Means, sample.Mean())
	res.MADs = append(res.MADs, sample.MAD())
	if res.Histogram != nil {
		if d.config.LogProfits.Normalize && sample.MAD() != 0.0 {
			sample, err = sample.Normalize()
			if err != nil {
				return errors.Annotate(err,
					"'%s': failed to normalize %s's log-profits", d.config.ID, ticker)
			}
		}
		res.Histogram.Add(sample.Data()...)
	}
	res.NumTickers++
	return nil
}

func (d *Distribution) processBatch(tickers []string) *jobResult {
	res := d.newJobResult()
	for _, t := range tickers {
		if err := d.processTicker(t, res); err != nil {
			res.Err = errors.Annotate(err, "failed to process ticker %s", t)
			return res
		}
	}
	return res
}

func (d *Distribution) Next() ([]string, bool) {
	if len(d.tickers) == 0 {
		return nil, false
	}
	batch := d.config.BatchSize
	if batch > len(d.tickers) {
		batch = len(d.tickers)
	}
	ts := d.tickers[:batch]
	d.tickers = d.tickers[batch:]
	return ts, true
}

func (d *Distribution) processTickers(tickers []string) error {
	d.tickers = tickers
	pm := iterator.ParallelMap[[]string, *jobResult](d.context, d.config.Workers, d, d.processBatch)
	res := iterator.Reduce(pm, d.newJobResult(), reduceJobResult)

	d.numTickers = res.NumTickers
	d.histogram = res.Histogram
	if d.config.Means != nil {
		c := d.config.Means
		d.meansHistogram = stats.NewHistogram(&c.Buckets)
		sample := stats.NewSample().Init(res.Means)
		if c.Normalize && sample.MAD() != 0.0 {
			var err error
			sample, err = sample.Normalize()
			if err != nil {
				return errors.Annotate(err,
					"'%s': failed to normalize means", d.config.ID)
			}
		}
		d.meansHistogram.Add(sample.Data()...)
	}
	if d.config.MADs != nil {
		c := d.config.MADs
		d.madsHistogram = stats.NewHistogram(&c.Buckets)
		sample := stats.NewSample().Init(res.MADs)
		if c.Normalize && sample.MAD() != 0.0 {
			var err error
			sample, err = sample.Normalize()
			if err != nil {
				return errors.Annotate(err,
					"'%s': failed to normalize MADs", d.config.ID)
			}
		}
		d.madsHistogram.Add(sample.Data()...)
	}
	return nil
}
