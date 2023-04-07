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
	"testing"

	"github.com/stockparfait/experiments"
	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/logging"
	"github.com/stockparfait/stockparfait/db"
	"github.com/stockparfait/stockparfait/stats"
	"github.com/stockparfait/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBuySellIntraday(t *testing.T) {
	t.Parallel()

	Convey("buy-sell intraday strategy", t, func() {
		ctx := context.Background()
		ctx = logging.Use(ctx, logging.DefaultGoLogger(logging.Info))

		Convey("buy at open, sell at target, stop loss or close", func() {
			var cfg config.BuySellIntradayStrategy
			js := testutil.JSON(`
{
  "buy": "9:00",
  "sell": [
    {"target": 1.02},
    {"stop loss": 0.95},
    {"time": "16:00"}
  ]
}`)
			So(cfg.InitMessage(js), ShouldBeNil)

			dates := []db.Date{
				dt("2020-01-01 09:00:00"), // buy at open
				dt("2020-01-01 12:00:00"), // no sell
				dt("2020-01-01 16:00:00"), // sell at close
				dt("2020-01-02 09:00:00"), // buy at open
				dt("2020-01-02 12:00:00"), // sell at target
				dt("2020-01-02 16:00:00"), // close, should not buy again
				dt("2020-01-03 09:00:00"), // buy at open
				dt("2020-01-03 12:00:00"), // sell on stop loss
				dt("2020-01-03 16:00:00"), // close, should not buy again
			}
			data := []float64{
				0.02, 0.01, -0.04, // first day -0.03, no target, no stop loss
				0.1, 0.02, -0.03, // second day: target at 12:00
				-0.1, -0.06, 0.3, // third day: stop loss at 12:00
			}

			lp := experiments.LogProfits{
				Ticker:     "TEST",
				Timeseries: stats.NewTimeseries(dates, data),
			}
			s := BuySellIntraday{config: &cfg}
			res := s.ExecuteTicker(ctx, lp, true)
			So(len(res.transactions), ShouldEqual, 6)
			So(res.transactions, ShouldResemble, []transaction{
				{buy: true, date: dt("2020-01-01 09:00:00"), amount: 1},
				{buy: false, date: dt("2020-01-01 16:00:00"), amount: 1},
				{buy: true, date: dt("2020-01-02 09:00:00"), amount: 1},
				{buy: false, date: dt("2020-01-02 12:00:00"), amount: 1},
				{buy: true, date: dt("2020-01-03 09:00:00"), amount: 1},
				{buy: false, date: dt("2020-01-03 12:00:00"), amount: 1},
			})
			So(testutil.Round(res.logProfit, 5), ShouldEqual, -0.03+0.02-0.06)
		})

		Convey("buy at open, sell at trailing stop loss, may keep overnight", func() {
			var cfg config.BuySellIntradayStrategy
			js := testutil.JSON(`
{
  "buy": "9:00",
  "sell": [
    {"stop loss trailing": 0.95}
  ]
}`)
			So(cfg.InitMessage(js), ShouldBeNil)

			dates := []db.Date{
				dt("2020-01-01 09:00:00"), // buy at open
				dt("2020-01-01 12:00:00"), // no sell
				dt("2020-01-01 16:00:00"), // close - no sell
				dt("2020-01-02 09:00:00"), // buy at open
				dt("2020-01-02 12:00:00"), // sell
				dt("2020-01-02 16:00:00"), // close, should not buy again
				dt("2020-01-03 09:00:00"), // buy at open
				dt("2020-01-03 12:00:00"), // no sell, but accounted in logProfit
			}
			data := []float64{
				0.02, -0.01, 0.02, // first day max drawdown -0.01, no stop loss
				0.1, -0.06, -0.03, // second day: stop loss at 12:00
				0.0, 0.01, // third day: keep position at the end
			}

			lp := experiments.LogProfits{
				Ticker:     "TEST",
				Timeseries: stats.NewTimeseries(dates, data),
			}
			s := BuySellIntraday{config: &cfg}
			res := s.ExecuteTicker(ctx, lp, true)
			So(len(res.transactions), ShouldEqual, 3)
			So(res.transactions, ShouldResemble, []transaction{
				{buy: true, date: dt("2020-01-01 09:00:00"), amount: 1},
				{buy: false, date: dt("2020-01-02 12:00:00"), amount: 1},
				{buy: true, date: dt("2020-01-03 09:00:00"), amount: 1},
			})
			So(testutil.Round(res.logProfit, 5), ShouldEqual, -0.01+0.02+0.1-0.06+0.01)
		})
	})
}
