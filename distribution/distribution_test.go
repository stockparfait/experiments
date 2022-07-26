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

package distribution

import (
	"context"
	"fmt"
	"io/ioutil"
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

func TestDistribution(t *testing.T) {
	t.Parallel()

	tmpdir, tmpdirErr := ioutil.TempDir("", "test_hold")
	defer os.RemoveAll(tmpdir)

	Convey("Test setup succeeded", t, func() {
		So(tmpdirErr, ShouldBeNil)
	})

	Convey("Distribution experiment works", t, func() {
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
				db.TestPrice(db.NewDate(2019, 1, 1), 10.0, 10.0, 1000.0, true),
				db.TestPrice(db.NewDate(2019, 1, 2), 12.0, 12.0, 1100.0, true),
				db.TestPrice(db.NewDate(2019, 1, 3), 11.0, 11.0, 1200.0, true),
			},
			"B": {
				db.TestPrice(db.NewDate(2019, 1, 1), 100.0, 100.0, 100.0, true),
				db.TestPrice(db.NewDate(2019, 1, 2), 120.0, 120.0, 110.0, true),
				db.TestPrice(db.NewDate(2019, 1, 3), 110.0, 110.0, 120.0, true),
			},
		}

		w := db.NewWriter(tmpdir, dbName)
		So(w.WriteTickers(tickers), ShouldBeNil)
		for t, p := range prices {
			So(w.WritePrices(t, p), ShouldBeNil)
		}

		g, err := canvas.EnsureGraph(plot.KindXY, "g", "dist")
		So(err, ShouldBeNil)
		sg, err := canvas.EnsureGraph(plot.KindXY, "sg", "dist")
		So(err, ShouldBeNil)

		Convey("defaults", func() {
			var cfg config.Distribution
			So(cfg.InitMessage(testutil.JSON(fmt.Sprintf(`{
  "data": {"DB path": "%s", "DB": "%s"},
  "graph": "g"
}`, tmpdir, dbName))), ShouldBeNil)
			var dist Distribution
			So(dist.Run(ctx, &cfg), ShouldBeNil)
			So(values, ShouldResemble, experiments.Values{
				"samples": "4",
				"tickers": "2",
			})
			So(len(sg.Plots), ShouldEqual, 0)
			So(len(g.Plots), ShouldEqual, 1)
		})

		Convey("with ID, analytical and samples", func() {
			var cfg config.Distribution
			So(cfg.InitMessage(testutil.JSON(fmt.Sprintf(`{
  "id": "test",
  "data": {"DB path": "%s", "DB": "%s"},
  "buckets": {"n": 3, "minval": -0.4, "maxval": 0.4},
  "use means": true,
  "graph": "g",
  "chart type": "bars",
  "normalize": false,
  "samples graph": "sg",
  "reference distribution": {"name": "t"}
}`, tmpdir, dbName))), ShouldBeNil)
			var dist Distribution
			So(dist.Run(ctx, &cfg), ShouldBeNil)
			So(values, ShouldResemble, experiments.Values{
				"test samples": "4",
				"test tickers": "2",
			})
			So(len(g.Plots), ShouldEqual, 2)
			So(len(sg.Plots), ShouldEqual, 1)
			// The first value is skipped due to 0 count.
			So(sg.Plots[0].Y, ShouldResemble, []float64{2, 2})
		})
	})
}
