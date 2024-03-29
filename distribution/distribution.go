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
	context context.Context
	config  *config.Distribution
}

var _ experiments.Experiment = &Distribution{}

func (d *Distribution) Prefix(s string) string {
	return experiments.Prefix(d.config.ID, s)
}

func (d *Distribution) AddValue(ctx context.Context, k, v string) error {
	return experiments.AddValue(ctx, d.config.ID, k, v)
}

func (d *Distribution) Run(ctx context.Context, cfg config.ExperimentConfig) error {
	var ok bool
	d.context = ctx
	if d.config, ok = cfg.(*config.Distribution); !ok {
		return errors.Reason("unexpected config type: %T", cfg)
	}
	id := d.config.ID
	it, err := experiments.SourceMap(ctx, d.config.Data, d.processLogProfits)
	if err != nil {
		return errors.Annotate(err, "failed to read data source")
	}
	defer it.Close()

	sts := iterator.Reduce[*jobResult, *jobResult](
		it, d.newJobResult(), reduceJobResult)

	if err := d.AddValue(ctx, "tickers", fmt.Sprintf("%d", sts.NumTickers)); err != nil {
		return errors.Annotate(err, "failed to add '%s' tickers value", id)
	}
	if sts.Histogram != nil {
		if err := d.AddValue(ctx, "samples", fmt.Sprintf("%d", sts.Histogram.CountsTotal())); err != nil {
			return errors.Annotate(err, "failed to add '%s' samples value", id)
		}
	}
	if sts.Histogram.CountsTotal() == 0 {
		return nil
	}
	if c := d.config.LogProfits; c != nil {
		lpDist := stats.NewHistogramDistribution(sts.Histogram)
		err := experiments.PlotDistribution(ctx, lpDist, c, id, "log-profit")
		if err != nil {
			return errors.Annotate(err, "failed to plot '%s' sample distribution", id)
		}
	}
	if c := d.config.Means; c != nil {
		meansDist := stats.NewSampleDistribution(sts.Means, &c.Buckets)
		err := experiments.PlotDistribution(ctx, meansDist, c, id, "means")
		if err != nil {
			return errors.Annotate(err, "failed to plot '%s' means", id)
		}
		err = d.AddValue(ctx, "average mean", fmt.Sprintf("%.4g", meansDist.Mean()))
		if err != nil {
			return errors.Annotate(err, "failed to add '%s' avg. mean", id)
		}
	}
	if c := d.config.MeanStability; c != nil && len(sts.MeanStability) > 1 {
		dist := stats.NewSampleDistribution(sts.MeanStability, &c.Plot.Buckets)
		err := experiments.PlotDistribution(ctx, dist, c.Plot, id, "mean stability")
		if err != nil {
			return errors.Annotate(err, "failed to plot '%s' mean stability", id)
		}
	}
	if c := d.config.MADs; c != nil {
		dist := stats.NewSampleDistribution(sts.MADs, &c.Buckets)
		err := experiments.PlotDistribution(ctx, dist, c, id, "MADs")
		if err != nil {
			return errors.Annotate(err, "failed to plot '%s' MADs distribution", id)
		}
		err = d.AddValue(ctx, "average MAD", fmt.Sprintf("%.4g", dist.Mean()))
		if err != nil {
			return errors.Annotate(err, "failed to add '%s' average MAD value", id)
		}
	}
	if c := d.config.MADStability; c != nil && len(sts.MADStability) > 1 {
		dist := stats.NewSampleDistribution(sts.MADStability, &c.Plot.Buckets)
		err := experiments.PlotDistribution(ctx, dist, c.Plot, id, "MAD stability")
		if err != nil {
			return errors.Annotate(err, "failed to plot '%s' MAD stability", id)
		}
	}
	return nil
}

type jobResult struct {
	Histogram     *stats.Histogram
	Means         []float64
	MADs          []float64
	MeanStability []float64
	MADStability  []float64
	NumTickers    int
}

func reduceJobResult(j, j2 *jobResult) *jobResult {
	if j.Histogram != nil {
		j.Histogram.AddHistogram(j2.Histogram)
	}
	j.Means = append(j.Means, j2.Means...)
	j.MADs = append(j.MADs, j2.MADs...)
	j.MeanStability = append(j.MeanStability, j2.MeanStability...)
	j.MADStability = append(j.MADStability, j2.MADStability...)
	j.NumTickers += j2.NumTickers
	return j
}

func (d *Distribution) newJobResult() *jobResult {
	res := &jobResult{}
	if d.config.LogProfits != nil {
		res.Histogram = stats.NewHistogram(&d.config.LogProfits.Buckets)
	}
	return res
}

func (d *Distribution) processLogProfits(lps []experiments.LogProfits) *jobResult {
	res := d.newJobResult()
	for _, lp := range lps {
		data := lp.Timeseries.Data()
		sample := stats.NewSample(data)
		res.Means = append(res.Means, sample.Mean())
		res.MADs = append(res.MADs, sample.MAD())
		meanF := func(l, h int) float64 { return stats.NewSample(data[l:h]).Mean() }
		MADF := func(l, h int) float64 { return stats.NewSample(data[l:h]).MAD() }
		res.MeanStability = append(res.MeanStability, experiments.Stability(
			len(data), meanF, d.config.MeanStability)...)
		res.MADStability = append(res.MADStability, experiments.Stability(
			len(data), MADF, d.config.MADStability)...)
		if res.Histogram != nil {
			if d.config.LogProfits.Normalize && sample.MAD() != 0.0 {
				var err error
				sample, err = sample.Normalize()
				if err != nil {
					logging.Warningf(d.context,
						"'%s': skipping %s, failed to normalize log-profits: %s",
						d.config.ID, lp.Ticker, err.Error())
					continue
				}
			}
			res.Histogram.Add(sample.Data()...)
		}
		res.NumTickers++
	}
	return res
}
