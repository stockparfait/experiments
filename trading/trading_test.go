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

package trading

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stockparfait/experiments"
	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/logging"
	"github.com/stockparfait/stockparfait/db"
	"github.com/stockparfait/stockparfait/plot"
	"github.com/stockparfait/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTrading(t *testing.T) {
	t.Parallel()

	tmpdir, tmpdirErr := os.MkdirTemp("", "test_trading")
	defer os.RemoveAll(tmpdir)

	Convey("Test setup succeeded", t, func() {
		So(tmpdirErr, ShouldBeNil)
	})

	pr := func(date string, o, h, c float64) db.PriceRow {
		d, err := db.NewDateFromString(date)
		if err != nil {
			panic(err)
		}
		return db.TestPriceRow(d, float32(c), float32(c), float32(c),
			float32(o), float32(h), float32(o), 1000.0, true)
	}

	Convey("Trading experiment works", t, func() {
		ctx := context.Background()
		ctx = logging.Use(ctx, logging.DefaultGoLogger(logging.Info))
		canvas := plot.NewCanvas()
		values := make(experiments.Values)
		ctx = plot.Use(ctx, canvas)
		ctx = experiments.UseValues(ctx, values)
		HOGraph, err := canvas.EnsureGraph(plot.KindXY, "ho", "group")
		So(err, ShouldBeNil)
		COGraph, err := canvas.EnsureGraph(plot.KindXY, "co", "group")
		So(err, ShouldBeNil)

		dbName := "db"
		tickers := map[string]db.TickerRow{
			"A": {},
			"B": {},
		}
		prices := map[string][]db.PriceRow{
			"A": {
				pr("2020-01-01", 100, 110, 90),
				pr("2020-01-02", 101, 101, 95),
				pr("2020-01-03", 102, 110, 105),
				pr("2020-01-04", 99, 100, 100),
				pr("2020-01-05", 100, 101, 80),
			},
			"B": {
				pr("2020-01-01", 100, 110, 90),
				pr("2020-01-02", 101, 101, 95),
				pr("2020-01-03", 102, 110, 105),
				pr("2020-01-04", 99, 100, 100),
				pr("2020-01-05", 100, 101, 80),
			},
		}
		w := db.NewWriter(tmpdir, dbName)
		So(w.WriteTickers(tickers), ShouldBeNil)
		for t, p := range prices {
			So(w.WritePrices(t, p), ShouldBeNil)
		}

		Convey("with all graphs", func() {
			var cfg config.Trading
			confJSON := fmt.Sprintf(`
{
  "id": "test",
  "data": {"DB": {
    "DB path": "%s",
    "DB": "%s"
  }},
  "high/open plot": {"graph": "ho"},
  "close/open plot": {"graph": "co"}
}`, tmpdir, dbName)
			So(cfg.InitMessage(testutil.JSON(confJSON)), ShouldBeNil)
			var tradingExp Trading
			So(tradingExp.Run(ctx, &cfg), ShouldBeNil)

			So(len(HOGraph.Plots), ShouldEqual, 1)
			So(len(COGraph.Plots), ShouldEqual, 1)
		})
	})

}
