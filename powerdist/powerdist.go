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
	"github.com/stockparfait/stockparfait/stats"
)

// PowerDist is an Experiment implementation for studying properties of
// analytical power and normal distributions.
type PowerDist struct {
	config   *config.PowerDist
	distName string
	source   stats.Distribution // true source distribution
	rand     *stats.RandDistribution
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
func randDistribution(ctx context.Context, c *config.AnalyticalDistribution) (source stats.Distribution, rand *stats.RandDistribution, name string, err error) {
	source, name, err = experiments.AnalyticalDistribution(ctx, c)
	if err != nil {
		err = errors.Annotate(err, "failed to create analytical distribution")
		return
	}
	var ok bool
	if rand, ok = source.(*stats.RandDistribution); !ok {
		xform := func(d stats.Distribution) float64 {
			return d.Rand()
		}
		rand = stats.NewRandDistribution(ctx, source, xform, c.Samples, &c.Buckets)
	}
	return
}

func (d *PowerDist) Run(ctx context.Context, cfg config.ExperimentConfig) error {
	var ok bool
	var err error
	if d.config, ok = cfg.(*config.PowerDist); !ok {
		return errors.Reason("unexpected config type: %T", cfg)
	}
	d.source, d.rand, d.distName, err = randDistribution(ctx, &d.config.Dist)
	if err != nil {
		return errors.Annotate(err, "failed to create RandDistribution")
	}
	if d.config.SamplePlot != nil {
		h := d.rand.Histogram()
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
	alphaFn := func() func(*stats.Histogram) float64 {
		// Add an extra function closure to cache and hide these vars.
		mean := d.source.Mean()
		mad := d.source.MAD()
		return func(h *stats.Histogram) float64 {
			return experiments.DeriveAlpha(h, mean, mad, d.config.AlphaParams)
		}
	}
	if err := d.plotStatistic(ctx, d.config.AlphaDist, alphaFn(), "Alphas"); err != nil {
		return errors.Annotate(err, "failed to plot '%s'", d.prefix("Alphas"))
	}

	var cumulMean, cumulMAD, cumulSigma, cumulAlpha *experiments.CumulativeStatistic
	if d.config.CumulMean != nil {
		cumulMean = experiments.NewCumulativeStatistic(d.config.CumulMean)
		cumulMean.SetExpected(d.source.Mean())
	}
	if d.config.CumulMAD != nil {
		cumulMAD = experiments.NewCumulativeStatistic(d.config.CumulMAD)
		cumulMAD.SetExpected(d.source.MAD())
	}
	if d.config.CumulSigma != nil {
		cumulSigma = experiments.NewCumulativeStatistic(d.config.CumulSigma)
		cumulSigma.SetExpected(math.Sqrt(d.source.Variance()))
	}
	if d.config.CumulAlpha != nil {
		cumulAlpha = experiments.NewCumulativeStatistic(d.config.CumulAlpha)
		cumulAlpha.SetExpected(d.config.Dist.Alpha)
	}

	cumulHist := stats.NewHistogram(&d.config.Dist.Buckets)
	for i := 0; i < d.config.CumulSamples; i++ {
		y := d.rand.Rand()
		cumulMean.AddToAverage(y)
		diff := d.config.Dist.Mean - y
		cumulMAD.AddToAverage(math.Abs(diff))
		cumulSigma.AddToAverage(diff * diff)
		cumulHist.Add(y)
		// Deriving alpha is expensive, skip if not needed.
		if cumulAlpha != nil {
			cumulAlpha.AddDirect(experiments.DeriveAlpha(
				cumulHist,
				d.config.Dist.Mean,
				d.config.Dist.MAD,
				d.config.AlphaParams,
			))
		}
	}
	cumulSigma.Map(func(y float64) float64 {
		if y < 0.0 {
			y = 0
		}
		return math.Sqrt(y)
	})

	if err := cumulMean.Plot(ctx, "mean", d.prefix("mean")); err != nil {
		return errors.Annotate(err, "failed to plot cumulative mean")
	}
	if err := cumulMAD.Plot(ctx, "MAD", d.prefix("MAD")); err != nil {
		return errors.Annotate(err, "failed to plot cumulative MAD")
	}
	if err := cumulSigma.Plot(ctx, "sigma", d.prefix("sigma")); err != nil {
		return errors.Annotate(err, "failed to plot cumulative sigma")
	}
	if err := cumulAlpha.Plot(ctx, "alpha", d.prefix("alpha")); err != nil {
		return errors.Annotate(err, "failed to plot cumulative alpha")
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
	_, dist, distName, err := randDistribution(ctx, &d.config.Dist)
	if err != nil {
		return errors.Annotate(err, "failed to create source distribution")
	}
	statDist := stats.NewRandDistribution(ctx, dist, xform, d.config.StatSamples, &c.Buckets)
	h := statDist.Histogram()
	fullName := d.prefix(distName + " " + name)
	if err = experiments.PlotDistribution(ctx, h, c, fullName); err != nil {
		return errors.Annotate(err, "failed to plot %s", fullName)
	}
	return nil
}
