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

// Package portfolio is a tool for analyzing the performance of an existing
// portfolio, implemented in a form of an "experiment".
package portfolio

import (
	"context"
	"fmt"
	"os"

	"github.com/stockparfait/errors"
	"github.com/stockparfait/experiments"
	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/stockparfait/db"
	"github.com/stockparfait/stockparfait/stats"
	"github.com/stockparfait/stockparfait/table"
)

// Portfolio is an Experiment implementation for analyzing an existing portfolio.
type Portfolio struct {
	config *config.Portfolio
}

var _ experiments.Experiment = &Portfolio{}

func (p *Portfolio) Prefix(s string) string {
	return experiments.Prefix(p.config.ID, s)
}

func (p *Portfolio) AddValue(ctx context.Context, k, v string) error {
	return experiments.AddValue(ctx, p.config.ID, k, v)
}

// Row of the output table as the list of strings compatible with encoding/csv.
type Row []string

var _ table.Row = Row{}

func (r Row) CSV() []string { return r }

func (p *Portfolio) Run(ctx context.Context, cfg config.ExperimentConfig) error {
	var ok bool
	if p.config, ok = cfg.(*config.Portfolio); !ok {
		return errors.Reason("unexpected config type: %T", cfg)
	}

	t := table.NewTable(p.header()...)
	for _, pos := range p.config.Positions {
		row, err := p.addPosition(ctx, pos)
		if err != nil {
			return errors.Annotate(err, "failed to add position for %s", pos.Ticker)
		}
		t.AddRow(row)
	}
	if err := p.writeTable(t); err != nil {
		return errors.Annotate(err, "failed to write positions table")
	}
	return nil
}

func (p *Portfolio) header() []string {
	r := make(Row, len(p.config.Columns))
	for i, c := range p.config.Columns {
		switch c.Kind {
		case "price", "value":
			r[i] = fmt.Sprintf("%s %s", c.Kind, c.Date)
		default:
			r[i] = c.Kind
		}
	}
	return r
}

// dataOnDate extracts data on the given date from the Timeseries, if present.
func dataOnDate(ts *stats.Timeseries, d db.Date) (float64, error) {
	day := ts.Range(d, d)
	if len(day.Data()) == 0 {
		return 0, errors.Reason("no price data for date %s", d)
	}
	return day.Data()[0], nil
}

func (p *Portfolio) addPosition(ctx context.Context, pos config.PortfolioPosition) (Row, error) {
	tr, err := p.config.Reader.TickerRow(pos.Ticker)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read ticker info for '%s'", pos.Ticker)
	}
	prices, err := p.config.Reader.Prices(pos.Ticker)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read prices for '%s'", pos.Ticker)
	}
	ts := stats.NewTimeseriesFromPrices(prices, stats.PriceSplitAdjusted)

	r := make(Row, len(p.config.Columns))
	for i, c := range p.config.Columns {
		switch c.Kind {
		case "ticker":
			r[i] = pos.Ticker
		case "name":
			r[i] = tr.Name
		case "exchange":
			r[i] = tr.Exchange
		case "category":
			r[i] = tr.Category
		case "sector":
			r[i] = tr.Sector
		case "industry":
			r[i] = tr.Industry
		case "purchase date":
			r[i] = pos.PurchaseDate.String()
		case "cost basis":
			cb := pos.CostBasis
			if cb == 0 {
				price, err := dataOnDate(ts, pos.PurchaseDate)
				if err != nil {
					return nil, errors.Annotate(err, "no cost basis and no price data")
				}
				cb = price * float64(pos.Shares)
			}
			r[i] = fmt.Sprintf("%.2f", cb)
		case "shares":
			r[i] = fmt.Sprintf("%d", pos.Shares)
		case "price":
			price, err := dataOnDate(ts, c.Date)
			if err != nil {
				return nil, errors.Annotate(err, "no price data")
			}
			r[i] = fmt.Sprintf("%.2f", price)
		case "value":
			price, err := dataOnDate(ts, c.Date)
			if err != nil {
				return nil, errors.Annotate(err, "no price data")
			}
			r[i] = fmt.Sprintf("%.2f", price*float64(pos.Shares))
		default:
			return nil, errors.Reason("unsupported column kind: '%s'", c.Kind)
		}
	}
	return r, nil
}

func (p *Portfolio) writeTable(t *table.Table) error {
	if p.config.File == "" {
		if err := t.WriteText(os.Stdout, table.Params{}); err != nil {
			return errors.Annotate(err, "failed to write table to stdout")
		}
	} else {
		f, err := os.OpenFile(p.config.File, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return errors.Annotate(err, "failed to open output CSV file '%s'", p.config.File)
		}
		defer f.Close()
		if err = t.WriteCSV(f, table.Params{}); err != nil {
			return errors.Annotate(err, "failed to write CSV file '%s'", p.config.File)
		}
	}
	return nil
}
