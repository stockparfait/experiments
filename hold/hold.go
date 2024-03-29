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

// Package hold implements a "buy and hold" experiment.
//
// Its primary purpose is to display price series for a set of stocks or a
// porftolio.
package hold

import (
	"context"
	"fmt"
	"sort"

	"github.com/stockparfait/errors"
	"github.com/stockparfait/experiments"
	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/stockparfait/db"
	"github.com/stockparfait/stockparfait/plot"
	"github.com/stockparfait/stockparfait/stats"
)

// Hold is the Experiment implentation for displaying price series of individual
// tickers and portfolios.
type Hold struct {
	config    *config.Hold
	positions []*stats.Timeseries
	total     *stats.Timeseries
}

var _ experiments.Experiment = &Hold{}

func (h *Hold) Prefix(s string) string {
	return experiments.Prefix(h.config.ID, s)
}

func (h *Hold) AddValue(ctx context.Context, k, v string) error {
	return experiments.AddValue(ctx, h.config.ID, k, v)
}

// Run implements experiments.Experiment.
func (h *Hold) Run(ctx context.Context, cfg config.ExperimentConfig) error {
	var ok bool
	if h.config, ok = cfg.(*config.Hold); !ok {
		return errors.Reason("unexpected config type: %T", cfg)
	}
	if h.config.PositionsGraph != "" {
		for _, p := range h.config.Positions {
			if err := h.AddPosition(ctx, p); err != nil {
				return errors.Annotate(err, "failed to add position for '%s'", p.Ticker)
			}
		}
	}
	if h.config.TotalGraph != "" {
		if err := h.AddTotal(ctx); err != nil {
			return errors.Annotate(err, "failed to add total")
		}
	}
	return nil
}

func (h *Hold) AddPosition(ctx context.Context, p config.HoldPosition) error {
	rows, err := h.config.Reader.Prices(p.Ticker)
	if err != nil {
		return errors.Annotate(err, "cannot load prices for '%s'", p.Ticker)
	}
	if len(rows) == 0 {
		return errors.Reason("no prices for '%s'", p.Ticker)
	}
	factor := p.Shares
	if factor == 0.0 {
		factor = p.StartValue / float64(rows[0].CloseFullyAdjusted)
	}
	dates := make([]db.Date, len(rows))
	data := make([]float64, len(rows))
	for i, r := range rows {
		dates[i] = r.Date
		data[i] = factor * float64(r.CloseFullyAdjusted)
	}
	ts := stats.NewTimeseries(dates, data)
	h.positions = append(h.positions, ts)

	legend := fmt.Sprintf("%.6g*%s", factor, p.Ticker)
	plt, err := plot.NewSeriesPlot(ts)
	if err != nil {
		return errors.Annotate(err, "failed to create plot '%s'", legend)
	}
	plt.SetYLabel("price").SetLegend(legend)
	if h.config.PositionsAxis == "left" {
		plt.SetLeftAxis(true)
	}
	err = plot.Add(ctx, plt, h.config.PositionsGraph)
	if err != nil {
		return errors.Annotate(err, "failed to add a position plot for '%s'",
			p.Ticker)
	}
	return nil
}

// AddTotal merges all the time series for positions pointwise. For simplicity,
// it uses the union of all dates, and considers missing price points as 0.0.
func (h *Hold) AddTotal(ctx context.Context) error {
	totalMap := make(map[db.Date]float64)
	for _, ps := range h.positions {
		for i, dt := range ps.Dates() {
			totalMap[dt] += ps.Data()[i]
		}
	}
	var keys []db.Date
	for k := range totalMap {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Before(keys[j]) })

	dates := make([]db.Date, len(keys))
	data := make([]float64, len(keys))
	for i, k := range keys {
		dates[i] = k
		data[i] = totalMap[k]
	}
	h.total = stats.NewTimeseries(dates, data)
	p, err := plot.NewSeriesPlot(h.total)
	if err != nil {
		return errors.Annotate(err, "failed to create plot 'Porftolio'")
	}
	p.SetYLabel("price").SetLegend("Portfolio")
	if h.config.TotalAxis == "left" {
		p.SetLeftAxis(true)
	}
	if err := plot.Add(ctx, p, h.config.TotalGraph); err != nil {
		return errors.Annotate(err, "failed to add a plot for portfolio total")
	}
	return nil
}
