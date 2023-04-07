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
	"github.com/stockparfait/stockparfait/plot"
	"github.com/stockparfait/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func dt(date string) db.Date {
	d, err := db.NewDateFromString(date)
	if err != nil {
		panic(err)
	}
	if d.IsZero() {
		panic("failed to parse date '" + date + "'")
	}
	return d
}

func TestSimulator(t *testing.T) {
	t.Parallel()

	Convey("Simulator experiment works", t, func() {
		ctx := context.Background()
		ctx = logging.Use(ctx, logging.DefaultGoLogger(logging.Info))
		canvas := plot.NewCanvas()
		values := make(experiments.Values)
		ctx = plot.Use(ctx, canvas)
		ctx = experiments.UseValues(ctx, values)
		profitGraph, err := canvas.EnsureGraph(plot.KindXY, "profit", "group")
		So(err, ShouldBeNil)

		Convey("runs a strategy", func() {
			var cfg config.Simulator
			confJSON := `
{
  "id": "test",
  "data": {
    "daily distribution": {"name": "t"},
    "intraday distribution": {"name": "t"},
    "intraday resolution": 30,
    "tickers": 1,
    "days": 10
  },
  "strategy": {"buy-sell intraday": {
    "buy": "9:30",
    "sell": [{"time": "15:30"}]
  }},
  "profit plot": {"graph": "profit"}
}`
			So(cfg.InitMessage(testutil.JSON(confJSON)), ShouldBeNil)
			var simExp Simulator
			So(simExp.Run(ctx, &cfg), ShouldBeNil)

			So(len(profitGraph.Plots), ShouldEqual, 1)
		})
	})
}
