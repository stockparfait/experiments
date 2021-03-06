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
	"github.com/stockparfait/stockparfait/plot"
	"github.com/stockparfait/stockparfait/stats"
)

// Distribution is an Experiment implementation for displaying and researching
// distributions of log-profits.
type Distribution struct {
	config     *config.Distribution
	context    context.Context
	histogram  *stats.Histogram
	numTickers int
	tickers    []string
}

var _ experiments.Experiment = &Distribution{}
var _ parallel.JobsIter = &Distribution{}

// maybeSkipZeros removes (x, y) elements where y < 1e-300, if so configured.
// Strictly speaking, we're trying to avoid zeros, but in practice anything
// below this number may be printed or interpreted as 0 in plots.
func (d *Distribution) maybeSkipZeros(xs, ys []float64) ([]float64, []float64) {
	if len(xs) != len(ys) {
		panic(errors.Reason("len(xs) [%d] != len(ys) [%d]", len(xs), len(ys)))
	}
	if d.config.KeepZeros {
		return xs, ys
	}
	xs1 := []float64{}
	ys1 := []float64{}
	for i, x := range xs {
		if ys[i] >= 1.0e-300 {
			xs1 = append(xs1, x)
			ys1 = append(ys1, ys[i])
		}
	}
	return xs1, ys1
}

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
	d.histogram = stats.NewHistogram(&d.config.Buckets)
	tickers, err := d.config.Reader.Tickers()
	if err != nil {
		return errors.Annotate(err, "failed to list tickers")
	}
	if err := d.processTickers(tickers); err != nil {
		return errors.Annotate(err, "failed to process tickers")
	}
	var xs0 []float64
	if d.config.UseMeans {
		xs0 = d.histogram.Xs()
	} else {
		xs0 = d.histogram.Buckets().Xs(0.5)
	}
	ys := d.histogram.PDFs()
	xs, ys := d.maybeSkipZeros(xs0, ys)
	plt := plot.NewXYPlot(xs, ys)
	plt.SetLegend(d.prefix("Sample p.d.f."))
	plt.SetYLabel("p.d.f.")
	if d.config.ChartType == "bars" {
		plt.SetChartType(plot.ChartBars)
	}
	if err := plot.Add(ctx, plt, d.config.Graph); err != nil {
		return errors.Annotate(err, "failed to add p.d.f. plot")
	}
	if err := d.plotAnalytical(ctx); err != nil {
		return errors.Annotate(err, "failed to plot analytical distribution")
	}
	if d.config.SamplesGraph != "" {
		ys := make([]float64, len(d.histogram.Counts()))
		for i, c := range d.histogram.Counts() {
			ys[i] = float64(c)
		}
		xs, ys := d.maybeSkipZeros(xs0, ys)
		plt := plot.NewXYPlot(xs, ys).SetLegend(d.prefix("Num samples"))
		plt.SetYLabel("count").SetLeftAxis(!d.config.SamplesRightAxis)
		if err := plot.Add(ctx, plt, d.config.SamplesGraph); err != nil {
			return errors.Annotate(err, "failed to add samples plot")
		}
	}
	if err := experiments.AddValue(ctx, d.prefix("tickers"), fmt.Sprintf("%d", d.numTickers)); err != nil {
		return errors.Annotate(err, "failed to add tickers value")
	}
	if err := experiments.AddValue(ctx, d.prefix("samples"), fmt.Sprintf("%d", d.histogram.Size())); err != nil {
		return errors.Annotate(err, "failed to add samples value")
	}
	return nil
}

type jobResult struct {
	Histogram  *stats.Histogram
	NumTickers int
	Err        error
}

func (d *Distribution) processTicker(ticker string, res *jobResult) error {
	rows, err := d.config.Reader.Prices(ticker)
	if err != nil {
		logging.Warningf(d.context, err.Error())
		return nil
	}
	if len(rows) == 0 {
		logging.Warningf(d.context, "no prices for %s", ticker)
		return nil
	}
	ts := stats.NewTimeseries().FromPrices(rows, stats.PriceFullyAdjusted)
	sample := ts.LogProfits()
	if d.config.Normalize && sample.MAD() != 0.0 {
		sample, err = sample.Normalize()
		if err != nil {
			return errors.Annotate(err, "failed to normalize log-profits")
		}
	}
	res.Histogram.Add(sample.Data()...)
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
		res := &jobResult{Histogram: stats.NewHistogram(d.histogram.Buckets())}
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
		d.histogram.AddHistogram(jr.Histogram)
		d.numTickers += jr.NumTickers
	}
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
		mad = d.histogram.MAD()
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
	var xs []float64
	if d.config.UseMeans {
		xs = d.histogram.Xs()
	} else {
		xs = d.histogram.Buckets().Xs(0.5)
	}
	ys := make([]float64, len(xs))
	for i, x := range xs {
		ys[i] = dist.Prob(x)
	}
	xs, ys = d.maybeSkipZeros(xs, ys)
	plt := plot.NewXYPlot(xs, ys)
	plt.SetLegend(d.prefix(distName)).SetChartType(plot.ChartDashed)
	plt.SetYLabel("p.d.f.")
	if err := plot.Add(ctx, plt, d.config.Graph); err != nil {
		return errors.Annotate(err, "failed to add analytical plot")
	}
	return nil
}
