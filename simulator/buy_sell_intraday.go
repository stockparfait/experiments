// Copyright 2023 Stock Parfait

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

//     http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package simulator

import (
	"context"

	"github.com/stockparfait/experiments"
	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/logging"
	"github.com/stockparfait/stockparfait/db"
)

// BuySellIntraday is a configurable day trading strategy.
type BuySellIntraday struct {
	config *config.BuySellIntradayStrategy
}

var _ Strategy = &BuySellIntraday{}

func (s BuySellIntraday) ExecuteTicker(ctx context.Context, lp experiments.LogProfits, xactions bool) strategyResult {
	var res strategyResult
	if len(lp.Timeseries.Data()) == 0 {
		logging.Warningf(ctx, "skipping %s: not enough price data", lp.Ticker)
		return res
	}
	var bought bool
	var tradedToday bool
	// Cumulative log-profit and the max. observed log-profit for the current
	// position, and the log-profit for the entire strategy.
	var logProfit, maxLogProfit, totalLogProfit float64
	for i, p := range lp.Timeseries.Data() {
		date := lp.Timeseries.Dates()[i]
		day := date.Date()
		if i == 0 {
			res.startDate = day
		}
		if day != res.endDate {
			tradedToday = false
		}
		res.endDate = day
		if bought {
			logProfit += p
			if logProfit > maxLogProfit {
				maxLogProfit = logProfit
			}
			if s.sell(date, logProfit, maxLogProfit) {
				bought = false
				tradedToday = true
				totalLogProfit += logProfit
				logProfit = 0
				maxLogProfit = 0
				if xactions {
					res.transactions = append(res.transactions, transaction{
						buy: false, date: date, amount: 1})
				}
			}
			continue
		}
		if s.buy(date, tradedToday) {
			logProfit = 0
			maxLogProfit = 0
			bought = true
			tradedToday = true
			if xactions {
				res.transactions = append(res.transactions, transaction{
					buy: true, date: date, amount: 1})
			}
		}
	}
	if bought {
		totalLogProfit += logProfit
	}
	res.logProfit = totalLogProfit
	return res
}

func (s BuySellIntraday) buy(date db.Date, tradedToday bool) bool {
	return !tradedToday && s.config.Buy <= date.Time
}

func (s BuySellIntraday) sell(date db.Date, logProfit, maxLogProfit float64) bool {
	for _, c := range s.config.Sell {
		switch {
		case c.Time != nil:
			if *c.Time <= date.Time {
				return true
			}
		case c.Target > 1:
			if logProfit >= c.LogTarget() {
				return true
			}
		case c.StopLoss > 0:
			if logProfit <= c.LogStopLoss() {
				return true
			}
		case c.StopLossTrailing > 0:
			if logProfit <= maxLogProfit+c.LogStopLossTrailing() {
				return true
			}
		}
	}
	return false
}
