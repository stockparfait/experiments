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

// Package powerdist is an experiment to study analytical power distributions.
package powerdist

import (
	"context"
	"math"

	"github.com/stockparfait/errors"
	"github.com/stockparfait/experiments"
	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/stockparfait/plot"
	"github.com/stockparfait/stockparfait/stats"
)

// PowerDist is an Experiment implementation for studying properties of
// analytical power and normal distributions.
type PowerDist struct {
	config   *config.PowerDist
	distName string
	dist     *stats.RandDistribution
}

var _ experiments.Experiment = &PowerDist{}

// prefix the experiment's ID to s, if there is one.
func (d *PowerDist) prefix(s string) string {
	if d.config.ID == "" {
		return s
	}
	return d.config.ID + " " + s
}

// randDistribution wraps analytical distribution into RandDistribution, as
// necessary.
func randDistribution(c *config.AnalyticalDistribution) (dist *stats.RandDistribution, name string, err error) {
	var d stats.Distribution
	d, name, err = experiments.AnalyticalDistribution(c)
	if err != nil {
		err = errors.Annotate(err, "failed to create analytical distribution")
		return
	}
	var ok bool
	if dist, ok = d.(*stats.RandDistribution); !ok {
		xform := func(d stats.Distribution) float64 {
			return d.Rand()
		}
		dist = stats.NewRandDistribution(d, xform, c.Samples, &c.Buckets)
	}
	return
}

func (d *PowerDist) Run(ctx context.Context, cfg config.ExperimentConfig) error {
	var ok bool
	var err error
	if d.config, ok = cfg.(*config.PowerDist); !ok {
		return errors.Reason("unexpected config type: %T", cfg)
	}
	d.dist, d.distName, err = randDistribution(&d.config.Dist)
	if err != nil {
		return errors.Annotate(err, "failed to create RandDistribution")
	}
	if d.config.SamplePlot != nil {
		h := d.dist.Histogram()
		c := d.config.SamplePlot
		name := d.prefix(d.distName)
		if err := experiments.PlotDistribution(ctx, h, c, name); err != nil {
			return errors.Annotate(err, "failed to plot %s", d.distName)
		}
	}
	meanFn := func(d *stats.Histogram) float64 { return d.Mean() }
	if err := d.plotStatistic(ctx, d.config.MeanDist, meanFn, "means"); err != nil {
		return errors.Annotate(err, "failed to plot '%s'", d.prefix("means"))
	}
	madFn := func(d *stats.Histogram) float64 { return d.MAD() }
	if err := d.plotStatistic(ctx, d.config.MADDist, madFn, "MADs"); err != nil {
		return errors.Annotate(err, "failed to plot '%s'", d.prefix("MADs"))
	}
	sigmaFn := func(d *stats.Histogram) float64 { return d.Sigma() }
	if err := d.plotStatistic(ctx, d.config.SigmaDist, sigmaFn, "Sigmas"); err != nil {
		return errors.Annotate(err, "failed to plot '%s'", d.prefix("Sigmas"))
	}

	// TODO: make it so all such cumulative statistics accumulate from the same
	// sample sequence. Rather than computing them separately, compute them at the
	// same time, accumulate only the points, so we don't have to save all the
	// samples, thus allowing potentially for billions of samples.
	//
	// To reuse point accumulation code, create a private struct type which will
	// accumulate the points + min & max, and then add plots as needed.
	nextMean := func() func() float64 {
		i := 0
		var sum float64
		return func() float64 {
			sum += d.dist.Rand()
			i++
			return sum / float64(i)
		}
	}()
	if err := d.plotStatsSamples(ctx, d.config.MeanGraph, "mean", nextMean, d.config.Samples, d.config.Points); err != nil {
		return errors.Annotate(err, "failed to plot means")
	}

	nextMAD := func() func() float64 {
		i := 0
		var sum float64
		return func() float64 {
			sum += math.Abs(d.config.Dist.Mean - d.dist.Rand())
			i++
			return sum / float64(i)
		}
	}()
	if err := d.plotStatsSamples(ctx, d.config.MADGraph, "MAD", nextMAD, d.config.Samples, d.config.Points); err != nil {
		return errors.Annotate(err, "failed to plot MADs")
	}

	nextSigma := func() func() float64 {
		i := 0
		var sum float64
		return func() float64 {
			x := d.config.Dist.Mean - d.dist.Rand()
			sum += x * x
			i++
			return math.Sqrt(sum / float64(i))
		}
	}()
	if err := d.plotStatsSamples(ctx, d.config.SigmaGraph, "sigma", nextSigma, d.config.Samples, d.config.Points); err != nil {
		return errors.Annotate(err, "failed to plot sigmas")
	}
	return nil
}

func (d *PowerDist) plotStatistic(
	ctx context.Context,
	c *config.DistributionPlot,
	f func(*stats.Histogram) float64, // compute the statistic
	name string,
) (err error) {
	if c == nil {
		return nil
	}
	xform := func(d stats.Distribution) float64 {
		rd := d.Copy().(*stats.RandDistribution) // use a fresh copy to recompute the histogram
		return f(rd.Histogram())
	}
	// Do NOT directly compute dist.Histogram() or statistics that require it, so
	// that copies would have to compute it every time.
	dist, distName, err := randDistribution(&d.config.Dist)
	if err != nil {
		return errors.Annotate(err, "failed to create source distribution")
	}
	statDist := stats.NewRandDistribution(dist, xform, d.config.Samples, &c.Buckets)
	h := statDist.Histogram()
	fullName := d.prefix(distName + " " + name)
	if err = experiments.PlotDistribution(ctx, h, c, fullName); err != nil {
		return errors.Annotate(err, "failed to plot %s", fullName)
	}
	return nil
}

func (d *PowerDist) plotStatsSamples(ctx context.Context, graph, name string, next func() float64, samples, points int) error {
	if graph == "" {
		return nil
	}
	var min, max float64
	var xs, ys, mins, maxs []float64
	point := func(i int) int {
		max := math.Log(float64(samples))
		x := max * float64(i+1) / float64(points)
		return int(math.Floor(math.Exp(x)))
	}
	nextPoint := point(0)

	for i := 0; i < samples; i++ {
		y := next()
		if i == 0 {
			min = y
			max = y
		}
		if y < min {
			min = y
		}
		if y > max {
			max = y
		}
		if i >= nextPoint {
			xs = append(xs, float64(i))
			ys = append(ys, y)
			mins = append(mins, min)
			maxs = append(maxs, max)
			min = y
			max = y
			nextPoint = point(len(xs))
		}
	}
	legend := d.prefix(name)
	plt, err := plot.NewXYPlot(xs, ys)
	if err != nil {
		return errors.Annotate(err, "failed to create plot '%s'", legend)
	}
	plt.SetLegend(legend).SetYLabel(name)
	if err := plot.Add(ctx, plt, graph); err != nil {
		return errors.Annotate(err, "failed to add plot '%s'", legend)
	}
	if !d.config.PlotMinMax {
		return nil
	}
	plt, err = plot.NewXYPlot(xs, mins)
	if err != nil {
		return errors.Annotate(err, "failed to create plot '%s min'", legend)
	}
	plt.SetLegend(legend + " min").SetYLabel(name).SetChartType(plot.ChartDashed)
	if err := plot.Add(ctx, plt, graph); err != nil {
		return errors.Annotate(err, "failed to add plot '%s min'", legend)
	}

	plt, err = plot.NewXYPlot(xs, maxs)
	if err != nil {
		return errors.Annotate(err, "failed to create plot '%s max'", legend)
	}
	plt.SetLegend(legend + " max").SetYLabel(name).SetChartType(plot.ChartDashed)
	if err := plot.Add(ctx, plt, graph); err != nil {
		return errors.Annotate(err, "failed to add plot '%s max'", legend)
	}
	return nil
}
