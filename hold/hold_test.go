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

package hold

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stockparfait/experiments"
	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/logging"
	"github.com/stockparfait/stockparfait/db"
	"github.com/stockparfait/stockparfait/plot"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHold(t *testing.T) {
	t.Parallel()
	tmpdir, tmpdirErr := ioutil.TempDir("", "test_hold")
	defer os.RemoveAll(tmpdir)

	Convey("Test setup succeeded", t, func() {
		So(tmpdirErr, ShouldBeNil)
	})

	Convey("Hold experiment works", t, func() {
		ctx := context.Background()
		ctx = logging.Use(ctx, logging.DefaultGoLogger(logging.Info))
		canvas := plot.NewCanvas()
		values := make(experiments.Values)
		ctx = plot.Use(ctx, canvas)
		ctx = experiments.UseValues(ctx, values)

		dbName := "db"
		tickers := map[string]db.TickerRow{
			"A": {},
			"B": {},
		}
		prices := map[string][]db.PriceRow{
			"A": {
				db.TestPrice(db.NewDate(2019, 1, 1), 10.0, 10.0, 10.0, 1000.0, true),
				db.TestPrice(db.NewDate(2019, 1, 2), 11.0, 11.0, 11.0, 1100.0, true),
				db.TestPrice(db.NewDate(2019, 1, 3), 12.0, 12.0, 12.0, 1200.0, true),
			},
			"B": {
				db.TestPrice(db.NewDate(2019, 1, 1), 100.0, 100.0, 100.0, 100.0, true),
				db.TestPrice(db.NewDate(2019, 1, 2), 110.0, 110.0, 110.0, 110.0, true),
				db.TestPrice(db.NewDate(2019, 1, 3), 120.0, 120.0, 120.0, 120.0, true),
			},
		}

		w := db.NewWriter(tmpdir, dbName)
		So(w.WriteTickers(tickers), ShouldBeNil)
		for t, p := range prices {
			So(w.WritePrices(t, p), ShouldBeNil)
		}
		So(w.WriteMetadata(w.Metadata), ShouldBeNil)

		pg, err := canvas.EnsureGraph(plot.KindSeries, "pg", "plots")
		So(err, ShouldBeNil)
		tg, err := canvas.EnsureGraph(plot.KindSeries, "tg", "plots")
		So(err, ShouldBeNil)

		cfg := &config.Hold{
			Reader: db.NewReader(tmpdir, dbName),
			Positions: []config.HoldPosition{
				{Ticker: "A", Shares: 2.0},
				{Ticker: "B", StartValue: 100.0},
			},
			PositionsGraph: "pg",
			TotalGraph:     "tg",
		}

		var h Hold
		So(h.Run(ctx, cfg), ShouldBeNil)
		So(pg.Plots, ShouldResemble, []*plot.Plot{
			{
				Kind:      plot.KindSeries,
				Dates:     []db.Date{db.NewDate(2019, 1, 1), db.NewDate(2019, 1, 2), db.NewDate(2019, 1, 3)},
				Y:         []float64{20, 22, 24},
				YLabel:    "price",
				Legend:    "2*A",
				ChartType: plot.ChartLine,
			},
			{
				Kind:      plot.KindSeries,
				Dates:     []db.Date{db.NewDate(2019, 1, 1), db.NewDate(2019, 1, 2), db.NewDate(2019, 1, 3)},
				Y:         []float64{100, 110, 120},
				YLabel:    "price",
				Legend:    "1*B",
				ChartType: plot.ChartLine,
			},
		})
		So(tg.Plots, ShouldResemble, []*plot.Plot{
			{
				Kind:      plot.KindSeries,
				Dates:     []db.Date{db.NewDate(2019, 1, 1), db.NewDate(2019, 1, 2), db.NewDate(2019, 1, 3)},
				Y:         []float64{120, 132, 144},
				YLabel:    "price",
				Legend:    "Portfolio",
				ChartType: plot.ChartLine,
			},
		})
	})
}
