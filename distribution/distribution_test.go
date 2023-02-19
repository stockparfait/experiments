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

	tmpdir, tmpdirErr := os.MkdirTemp("", "test_distribution")
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
				db.TestPrice(db.NewDate(2019, 1, 1), 10.0, 10.0, 10.0, 1000.0, true),
				db.TestPrice(db.NewDate(2019, 1, 2), 12.0, 12.0, 12.0, 1100.0, true),
				db.TestPrice(db.NewDate(2019, 1, 3), 11.0, 11.0, 11.0, 1200.0, true),
			},
			"B": {
				db.TestPrice(db.NewDate(2019, 1, 1), 100.0, 100.0, 100.0, 100.0, true),
				db.TestPrice(db.NewDate(2019, 1, 2), 120.0, 120.0, 120.0, 110.0, true),
				db.TestPrice(db.NewDate(2019, 1, 3), 110.0, 110.0, 110.0, 120.0, true),
			},
		}

		w := db.NewWriter(tmpdir, dbName)
		So(w.WriteTickers(tickers), ShouldBeNil)
		for t, p := range prices {
			So(w.WritePrices(t, p), ShouldBeNil)
		}

		g, err := canvas.EnsureGraph(plot.KindXY, "g", "dist")
		So(err, ShouldBeNil)

		Convey("defaults", func() {
			var cfg config.Distribution
			So(cfg.InitMessage(testutil.JSON(fmt.Sprintf(`{
  "data": {"DB path": "%s", "DB": "%s"},
  "log-profits": {"graph": "g"}
}`, tmpdir, dbName))), ShouldBeNil)
			var dist Distribution
			So(dist.Run(ctx, &cfg), ShouldBeNil)
			So(values, ShouldResemble, experiments.Values{
				"log-profit P(X < mean-10*sigma)": "0.5481",
				"log-profit P(X > mean+10*sigma)": "0.4519",
				"samples":                         "4",
				"tickers":                         "2",
			})
			So(len(g.Plots), ShouldEqual, 1)
		})

		Convey("non-default parameters", func() {
			var cfg config.Distribution
			So(cfg.InitMessage(testutil.JSON(fmt.Sprintf(`{
  "id": "test",
  "data": {"DB path": "%s", "DB": "%s"},
  "log-profits": {
    "graph": "g",
    "counts graph": "g",
    "buckets": {"n": 3, "min": -0.4, "max": 0.4},
    "normalize": false,
    "use means": true,
    "log Y": true,
    "chart type": "bars",
    "plot mean": true,
    "percentiles": [5, 95],
    "reference distribution": {"analytical source": {"name": "t"}}
  },
  "means": {"graph": "g"},
  "MADs": {"graph": "g"},
  "mean stability": {"plot": {"graph": "g"}},
  "MAD stability": {"plot": {"graph": "g"}}
}`, tmpdir, dbName))), ShouldBeNil)
			var dist Distribution
			So(dist.Run(ctx, &cfg), ShouldBeNil)
			So(values, ShouldResemble, experiments.Values{
				"test samples":                             "4",
				"test tickers":                             "2",
				"test average MAD":                         "0.1347",
				"test MADs P(X < mean-10*sigma)":           "0.636",
				"test MADs P(X > mean+10*sigma)":           "0.364",
				"test MAD stability P(X < mean-10*sigma)":  "1",
				"test MAD stability P(X > mean+10*sigma)":  "0",
				"test average mean":                        "0.04766",
				"test means P(X < mean-10*sigma)":          "0.5481",
				"test means P(X > mean+10*sigma)":          "0.4519",
				"test mean stability P(X < mean-10*sigma)": "0",
				"test mean stability P(X > mean+10*sigma)": "0",
				"test log-profit mean":                     "0.04766",
				"test log-profit MAD":                      "0.1347",
				"test log-profit alpha":                    "3",
				"test log-profit P(X < mean-10*sigma)":     "0",
				"test log-profit P(X > mean+10*sigma)":     "0"})
			So(len(g.Plots), ShouldEqual, 10)
			So(g.Plots[1].Legend, ShouldEqual, "test log-profit counts")
			// The first value is skipped due to 0 count.
			So(g.Plots[1].Y, ShouldResemble, []float64{2, 2})
		})
	})
}
