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
	"math"
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
		d := func(date string) db.Date {
			res, err := db.NewDateFromString(date)
			if err != nil {
				panic(err)
			}
			return res
		}
		pr := func(date string, p float64) db.PriceRow {
			return db.TestPrice(d(date), float32(p), float32(p), float32(p), 1000.0, true)
		}

		prices := map[string][]db.PriceRow{
			"A": {
				pr("2019-01-01", 10.0),
				pr("2019-01-02", 10.0*math.Exp(0.1)),
				pr("2019-01-03", 10.0*math.Exp(0.1-0.05)),
			},
			"B": {
				pr("2019-01-01", 100.0),
				pr("2019-01-02", 100.0*math.Exp(-0.1)),
				pr("2019-01-03", 100.0*math.Exp(-0.1+0.05)),
			},
		}

		w := db.NewWriter(tmpdir, dbName)
		So(w.WriteTickers(tickers), ShouldBeNil)
		for t, p := range prices {
			So(w.WritePrices(t, p), ShouldBeNil)
		}

		distGraph, err := canvas.EnsureGraph(plot.KindXY, "dist", "gr")
		So(err, ShouldBeNil)
		distCountsGraph, err := canvas.EnsureGraph(plot.KindXY, "dist counts", "gr")
		So(err, ShouldBeNil)
		meansGraph, err := canvas.EnsureGraph(plot.KindXY, "means", "gr")
		So(err, ShouldBeNil)
		madsGraph, err := canvas.EnsureGraph(plot.KindXY, "mads", "gr")
		So(err, ShouldBeNil)
		meansStabGraph, err := canvas.EnsureGraph(plot.KindXY, "means stab", "gr")
		So(err, ShouldBeNil)
		madsStabGraph, err := canvas.EnsureGraph(plot.KindXY, "mads stab", "gr")
		So(err, ShouldBeNil)

		Convey("DB with default parameters", func() {
			var cfg config.Distribution
			So(cfg.InitMessage(testutil.JSON(fmt.Sprintf(`{
  "data": {"DB": {"DB path": "%s", "DB": "%s"}},
  "log-profits": {"graph": "dist"}
}`, tmpdir, dbName))), ShouldBeNil)
			var dist Distribution
			So(dist.Run(ctx, &cfg), ShouldBeNil)
			So(values, ShouldResemble, experiments.Values{
				"log-profit P(X < mean-10*sigma)": "0.5",
				"log-profit P(X > mean+10*sigma)": "0.5",
				"samples":                         "4",
				"tickers":                         "2",
			})
			So(len(distGraph.Plots), ShouldEqual, 1)
		})

		Convey("DB with custom parameters", func() {
			var cfg config.Distribution
			So(cfg.InitMessage(testutil.JSON(fmt.Sprintf(`{
  "id": "test",
  "data": {
    "DB": {"DB path": "%s", "DB": "%s"},
    "workers": 1
  },
  "log-profits": {
    "graph": "dist",
    "counts graph": "dist counts",
    "buckets": {"n": 3, "min": -0.1, "max": 0.1},
    "normalize": false,
    "use means": true,
    "log Y": true,
    "chart type": "bars",
    "plot mean": true,
    "percentiles": [5, 95],
    "reference distribution": {"analytical source": {"name": "t"}}
  },
  "means": {"graph": "means"},
  "MADs": {"graph": "mads"},
  "mean stability": {"plot": {"graph": "means stab"}},
  "MAD stability": {"plot": {"graph": "mads stab"}}
}`, tmpdir, dbName))), ShouldBeNil)
			var dist Distribution
			So(dist.Run(ctx, &cfg), ShouldBeNil)
			So(values["test samples"], ShouldEqual, "4")
			So(values["test tickers"], ShouldEqual, "2")
			// So(values["test average mean"], ShouldEqual, "0.04766")
			So(values["test average MAD"], ShouldEqual, "0.075")
			// So(values["test log-profit mean"], ShouldEqual, "0.04766")
			So(values["test log-profit MAD"], ShouldEqual, "0.075")
			So(values["test log-profit alpha"], ShouldEqual, "3")
			So(len(distGraph.Plots), ShouldEqual, 5) // dist, mean, 2x %-iles, ref
			So(len(distCountsGraph.Plots), ShouldEqual, 1)
			So(distCountsGraph.Plots[0].Legend, ShouldEqual, "test log-profit counts")
			// The first value is skipped due to 0 count.
			So(distCountsGraph.Plots[0].Y, ShouldResemble, []float64{2, 2})
			So(len(meansGraph.Plots), ShouldEqual, 1)
			So(testutil.RoundSlice(meansGraph.Plots[0].Y, 5), ShouldResemble, []float64{
				1010, 1010})
			So(len(madsGraph.Plots), ShouldEqual, 1)
			So(len(meansStabGraph.Plots), ShouldEqual, 1)
			So(len(madsStabGraph.Plots), ShouldEqual, 1)
		})
	})
}
