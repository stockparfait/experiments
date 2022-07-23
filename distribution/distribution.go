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
	"github.com/stockparfait/stockparfait/plot"
	"github.com/stockparfait/stockparfait/stats"
)

// Distribution is an Experiment implementation for displaying and researching
// distributions of log-profits.
type Distribution struct {
	config     *config.Distribution
	histogram  *stats.Histogram
	numTickers int
	numSamples int
}

var _ experiments.Experiment = &Distribution{}

func (d *Distribution) Run(ctx context.Context, cfg config.ExperimentConfig) error {
	var ok bool
	if d.config, ok = cfg.(*config.Distribution); !ok {
		return errors.Reason("unexpected config type: %T", cfg)
	}
	d.histogram = stats.NewHistogram(&d.config.Buckets)
	tickers, err := d.config.Reader.Tickers()
	if err != nil {
		return errors.Annotate(err, "failed to list tickers")
	}
	d.numTickers = len(tickers)
	for _, t := range tickers {
		if err := d.processTicker(t); err != nil {
			return errors.Annotate(err, "failed to process ticker '%s'", t)
		}
	}
	plt := plot.NewXYPlot(d.histogram.Buckets().Xs(0.5), d.histogram.PDFs())
	plt.SetLegend("Sample p.d.f.").SetChartType(plot.ChartBars)
	plt.SetYLabel("p.d.f.")
	plot.AddRight(ctx, plt, d.config.Graph)
	if err := d.plotAnalytical(ctx); err != nil {
		return errors.Annotate(err, "failed to plot analytical distribution")
	}
	if err := experiments.AddValue(ctx, "tickers", fmt.Sprintf("%d", d.numTickers)); err != nil {
		return errors.Annotate(err, "failed to add tickers value")
	}
	if err := experiments.AddValue(ctx, "samples", fmt.Sprintf("%d", d.numSamples)); err != nil {
		return errors.Annotate(err, "failed to add samples value")
	}
	return nil
}

func (d *Distribution) processTicker(ticker string) error {
	rows, err := d.config.Reader.Prices(ticker)
	if err != nil {
		return errors.Annotate(err, "cannot load prices for '%s'", ticker)
	}
	ts := stats.NewTimeseries().FromPrices(rows, stats.PriceFullyAdjusted)
	sample := ts.LogProfits()
	if d.config.Normalize {
		sample, err = sample.Normalize()
		if err != nil {
			return errors.Annotate(err, "failed to normalize log-profits")
		}
	}
	d.histogram.Add(sample.Data()...)
	d.numSamples += len(sample.Data())
	return nil
}

func (d *Distribution) plotAnalytical(ctx context.Context) error {
	if d.config.RefDist == nil {
		return nil
	}
	mean := d.config.RefDist.Mean
	mad := d.config.RefDist.MAD
	if !d.config.Normalize {
		mean = d.histogram.Mean()
		// TODO: mad = d.histogram.MAD()
	}
	var dist stats.Distribution
	distName := ""
	switch d.config.RefDist.Name {
	case "t":
		dist = stats.NewStudentsTDistribution(d.config.RefDist.Alpha, mean, mad)
		distName = "Student's T distribution"
	case "normal":
		dist = stats.NewNormalDistribution(mean, mad)
		distName = "Normal distribution"
	default:
		return errors.Reason("unsuppoted distribution type: '%s'",
			d.config.RefDist.Name)
	}
	xs := d.histogram.Buckets().Xs(0.5)
	ys := make([]float64, len(xs))
	for i, x := range xs {
		ys[i] = dist.Prob(x)
	}
	plt := plot.NewXYPlot(xs, ys)
	plt.SetLegend(distName).SetChartType(plot.ChartLine)
	plt.SetYLabel("p.d.f.")
	plot.AddRight(ctx, plt, d.config.Graph)
	return nil
}
