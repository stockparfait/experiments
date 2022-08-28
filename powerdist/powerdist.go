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

	"github.com/stockparfait/errors"
	"github.com/stockparfait/experiments"
	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/stockparfait/stats"
)

// PowerDist is an Experiment implementation for studying properties of analytical power distributions.
type PowerDist struct {
	config   *config.PowerDist
	context  context.Context
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
	d.context = ctx
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
