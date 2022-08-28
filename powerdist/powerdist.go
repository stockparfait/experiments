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
	meanDist *stats.RandDistribution
	// madDist   *stats.RandDistribution
	// sigmaDist *stats.RandDistribution
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
	if err := d.plotMeans(ctx); err != nil {
		return errors.Annotate(err, "failed to plot '%s'", d.prefix("means"))
	}
	return nil
}

func (d *PowerDist) plotMeans(ctx context.Context) (err error) {
	if d.config.MeanDist == nil {
		return nil
	}
	c := d.config.MeanDist
	xform := func(d stats.Distribution) float64 {
		return d.Copy().Mean() // to use a fresh seed and recompute mean
	}
	dist, _, err := randDistribution(&d.config.Dist)
	if err != nil {
		return errors.Annotate(err, "failed to create source distribution")
	}
	d.meanDist = stats.NewRandDistribution(dist, xform, d.config.Samples, &c.Buckets)
	err = experiments.PlotDistribution(
		ctx, d.meanDist.Histogram(), c, d.prefix("means"))
	if err != nil {
		return errors.Annotate(err, "failed to plot %s", d.prefix("means"))
	}
	return nil
}
