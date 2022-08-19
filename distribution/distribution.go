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
	"math"

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

// maybeSkipZeros removes (x, y) elements where y < 1e-300, if so configured.
// Strictly speaking, we're trying to avoid zeros, but in practice anything
// below this number may be printed or interpreted as 0 in plots.
func maybeSkipZeros(xs, ys []float64, c *config.DistributionPlot) ([]float64, []float64) {
	if len(xs) != len(ys) {
		panic(errors.Reason("len(xs) [%d] != len(ys) [%d]", len(xs), len(ys)))
	}
	if c.KeepZeros {
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

// maybeLog10 computes log10 for the slice of values if LogPDF is true.
func maybeLog10(ys []float64, c *config.DistributionPlot) []float64 {
	if !c.LogY {
		return ys
	}
	res := make([]float64, len(ys))
	for i, y := range ys {
		res[i] = math.Log10(y)
	}
	return res
}

// filterXY optionally skips zeros, computes log10 if configured, and stores the
// Y range for vertical lines.
func filterXY(xs, ys []float64, c *config.DistributionPlot) ([]float64, []float64) {
	xs, ys = maybeSkipZeros(xs, ys, c)
	ys = maybeLog10(ys, c)
	return xs, ys
}

// minMax returns the min and max values from ys.
func minMax(ys []float64) (float64, float64) {
	min := math.Inf(1)
	max := math.Inf(-1)
	for _, y := range ys {
		if y < min {
			min = y
		}
		if y > max {
			max = y
		}
	}
	return min, max
}

func plotDistribution(ctx context.Context, h *stats.Histogram, c *config.DistributionPlot, legend string) error {
	if c == nil {
		return nil
	}
	var xs []float64
	var ys []float64
	yLabel := "p.d.f."

	if c.UseMeans {
		xs = h.Xs()
	} else {
		xs = h.Buckets().Xs(0.5)
	}

	if c.RawCounts {
		yLabel = "counts"
		ys = make([]float64, len(h.Counts()))
		for i, c := range h.Counts() {
			ys[i] = float64(c)
		}
	} else {
		ys = h.PDFs()
	}
	xs, ys = filterXY(xs, ys, c)
	min, max := minMax(ys)
	plt, err := plot.NewXYPlot(xs, ys)
	if err != nil {
		return errors.Annotate(err, "failed to create plot '%s'", legend)
	}
	plt.SetLegend(legend + " " + yLabel)
	if c.LogY {
		yLabel = "log10(" + yLabel + ")"
	}
	plt.SetYLabel(yLabel)
	if c.ChartType == "bars" {
		plt.SetChartType(plot.ChartBars)
	}
	plt.SetLeftAxis(c.LeftAxis)
	if err := plot.Add(ctx, plt, c.Graph); err != nil {
		return errors.Annotate(err, "failed to add plot '%s'", legend)
	}
	if c.PlotMean {
		if err := plotMean(ctx, h, c.Graph, min, max, legend); err != nil {
			return errors.Annotate(err, "failed to plot '%s mean'", legend)
		}
	}
	if err := plotPercentiles(ctx, h, c, min, max, legend); err != nil {
		return errors.Annotate(err, "failed to plot '%s percentiles'", legend)
	}
	if err := plotAnalytical(ctx, h, c, legend); err != nil {
		return errors.Annotate(err, "failed to plot '%s ref dist'", legend)
	}
	return nil
}

func plotMean(ctx context.Context, h *stats.Histogram, graph string, min, max float64, legend string) error {
	x := h.Mean()
	plt, err := plot.NewXYPlot([]float64{x, x}, []float64{min, max})
	if err != nil {
		return errors.Annotate(err, "failed to create plot '%s mean'", legend)
	}
	plt.SetLegend(fmt.Sprintf("%s mean=%.4g", legend, x))
	plt.SetYLabel("").SetChartType(plot.ChartDashed)
	if err := plot.Add(ctx, plt, graph); err != nil {
		return errors.Annotate(err, "failed to add '%s mean' plot", legend)
	}
	return nil
}

func plotPercentiles(ctx context.Context, h *stats.Histogram, c *config.DistributionPlot, min, max float64, legend string) error {
	for _, p := range c.Percentiles {
		x := h.Quantile(p / 100.0)
		plt, err := plot.NewXYPlot([]float64{x, x}, []float64{min, max})
		if err != nil {
			return errors.Annotate(err, "failed to create plot '%s %gth %%-ile'",
				legend, p)
		}
		plt.SetLegend(fmt.Sprintf("%s %gth %%-ile=%.3g", legend, p, x))
		plt.SetYLabel("").SetChartType(plot.ChartDashed)
		if err := plot.Add(ctx, plt, c.Graph); err != nil {
			return errors.Annotate(err, "failed to add plot '%s %gth %%-ile'", legend, p)
		}
	}
	return nil
}

func plotAnalytical(ctx context.Context, h *stats.Histogram, c *config.DistributionPlot, legend string) error {
	if c.RefDist == nil {
		return nil
	}
	mean := c.RefDist.Mean
	mad := c.RefDist.MAD
	if c.AdjustRef {
		mean = h.Mean()
		mad = h.MAD()
	}
	var dist stats.Distribution
	distName := ""
	switch c.RefDist.Name {
	case "t":
		dist = stats.NewStudentsTDistribution(c.RefDist.Alpha, mean, mad)
		distName = fmt.Sprintf("T distribution a=%.2f", c.RefDist.Alpha)
	case "normal":
		dist = stats.NewNormalDistribution(mean, mad)
		distName = "Normal distribution"
	default:
		return errors.Reason("unsuppoted distribution type: '%s'", c.RefDist.Name)
	}
	var xs []float64
	if c.UseMeans {
		xs = h.Xs()
	} else {
		xs = h.Buckets().Xs(0.5)
	}
	ys := make([]float64, len(xs))
	for i, x := range xs {
		ys[i] = dist.Prob(x)
	}
	xs, ys = filterXY(xs, ys, c)
	plt, err := plot.NewXYPlot(xs, ys)
	if err != nil {
		return errors.Annotate(err, "failed to create '%s' analytical plot", legend)
	}
	plt.SetLegend(legend + " " + distName).SetChartType(plot.ChartDashed)
	if c.LogY {
		plt.SetYLabel("log10(p.d.f.)")
	} else {
		plt.SetYLabel("p.d.f.")
	}
	if err := plot.Add(ctx, plt, c.Graph); err != nil {
		return errors.Annotate(err, "failed to add '%s' analytical plot", legend)
	}
	return nil
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
	if err := plotDistribution(ctx, d.histogram, d.config.LogProfits, d.prefix("log-profit")); err != nil {
		return errors.Annotate(err, "failed to plot '%s' sample distribution", d.config.ID)
	}
	if err := plotDistribution(ctx, d.meansHistogram, d.config.Means, d.prefix("means")); err != nil {
		return errors.Annotate(err, "failed to plot '%s' means distribution", d.config.ID)
	}
	if err := plotDistribution(ctx, d.madsHistogram, d.config.MADs, d.prefix("MADs")); err != nil {
		return errors.Annotate(err, "failed to plot '%s' MADs distribution", d.config.ID)
	}
	if err := experiments.AddValue(ctx, d.prefix("tickers"), fmt.Sprintf("%d", d.numTickers)); err != nil {
		return errors.Annotate(err, "failed to add '%s' tickers value", d.config.ID)
	}
	if d.histogram != nil {
		if err := experiments.AddValue(ctx, d.prefix("samples"), fmt.Sprintf("%d", d.histogram.Size())); err != nil {
			return errors.Annotate(err, "failed to add '%s' samples value", d.config.ID)
		}
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
