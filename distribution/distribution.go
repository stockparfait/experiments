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
	"github.com/stockparfait/logging"
	"github.com/stockparfait/parallel"
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
var _ parallel.JobsIter = &Distribution{}

// prefix the experiment's ID to s, if there is one.
func (d *Distribution) prefix(s string) string {
	if d.config.ID == "" {
		return s
	}
	return d.config.ID + " " + s
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
	if err := experiments.AddValue(ctx, d.prefix("tickers"), fmt.Sprintf("%d", d.numTickers)); err != nil {
		return errors.Annotate(err, "failed to add '%s' tickers value", d.config.ID)
	}
	if d.histogram != nil {
		if err := experiments.AddValue(ctx, d.prefix("samples"), fmt.Sprintf("%d", d.histogram.Size())); err != nil {
			return errors.Annotate(err, "failed to add '%s' samples value", d.config.ID)
		}
	}
	if d.histogram.Size() == 0 {
		return nil
	}
	if err := experiments.PlotDistribution(ctx, stats.NewHistogramDistribution(d.histogram), d.config.LogProfits, d.prefix("log-profit")); err != nil {
		return errors.Annotate(err, "failed to plot '%s' sample distribution", d.config.ID)
	}
	if err := experiments.PlotDistribution(ctx, stats.NewHistogramDistribution(d.meansHistogram), d.config.Means, d.prefix("means")); err != nil {
		return errors.Annotate(err, "failed to plot '%s' means distribution", d.config.ID)
	}
	if err := experiments.PlotDistribution(ctx, stats.NewHistogramDistribution(d.madsHistogram), d.config.MADs, d.prefix("MADs")); err != nil {
		return errors.Annotate(err, "failed to plot '%s' MADs distribution", d.config.ID)
	}
	if d.meansHistogram != nil {
		if err := experiments.AddValue(ctx, d.prefix("average mean"), fmt.Sprintf("%.4g", d.meansHistogram.Mean())); err != nil {
			return errors.Annotate(err,
				"failed to add '%s' average mean value", d.config.ID)
		}
	}
	if d.madsHistogram != nil {
		if err := experiments.AddValue(ctx, d.prefix("average MAD"), fmt.Sprintf("%.4g", d.madsHistogram.Mean())); err != nil {
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

// Next implements parallel.JobsIter for processing tickers.
func (d *Distribution) Next() (parallel.Job, error) {
	if len(d.tickers) == 0 {
		return nil, parallel.Done
	}
	batch := d.config.BatchSize
	if batch > len(d.tickers) {
		batch = len(d.tickers)
	}
	ts := d.tickers[:batch]
	d.tickers = d.tickers[batch:]
	f := func() interface{} {
		res := &jobResult{}
		if d.histogram != nil {
			res.Histogram = stats.NewHistogram(d.histogram.Buckets())
		}
		for _, t := range ts {
			if err := d.processTicker(t, res); err != nil {
				res.Err = errors.Annotate(err, "failed to process ticker %s", t)
				return res
			}
		}
		return res
	}
	return f, nil
}

func (d *Distribution) processTickers(tickers []string) error {
	d.tickers = tickers
	pm := parallel.Map(d.context, d.config.Workers, d)

	var means []float64
	var mads []float64
	for {
		v, err := pm.Next()
		if err == parallel.Done {
			break
		}
		if err != nil {
			return errors.Annotate(err, "failed to process tickers")
		}
		jr, ok := v.(*jobResult)
		if !ok {
			return errors.Reason("unexpected result: %T", v)
		}
		if jr.Err != nil {
			return errors.Annotate(jr.Err, "job failed")
		}
		if d.histogram != nil {
			d.histogram.AddHistogram(jr.Histogram)
		}
		means = append(means, jr.Means...)
		mads = append(mads, jr.MADs...)
		d.numTickers += jr.NumTickers
	}
	if d.config.Means != nil {
		c := d.config.Means
		d.meansHistogram = stats.NewHistogram(&c.Buckets)
		sample := stats.NewSample().Init(means)
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
		sample := stats.NewSample().Init(mads)
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
