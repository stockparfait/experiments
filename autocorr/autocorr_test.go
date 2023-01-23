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

package autocorr

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

func TestAutoCorrelation(t *testing.T) {
	t.Parallel()

	tmpdir, tmpdirErr := os.MkdirTemp("", "test_autocorr")
	defer os.RemoveAll(tmpdir)

	Convey("Test setup succeeded", t, func() {
		So(tmpdirErr, ShouldBeNil)
	})

	Convey("AutoCorrelation works", t, func() {
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
				db.TestPrice(db.NewDate(2019, 1, 4), 11.0, 11.0, 10.0, 1200.0, true),
				db.TestPrice(db.NewDate(2019, 1, 5), 11.0, 11.0, 13.0, 1200.0, true),
			},
			"B": { // insufficient data, will be skipped
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

		Convey("with shift [1..2]", func() {
			var cfg config.AutoCorrelation
			confJSON := fmt.Sprintf(`
{
  "data": {"DB path": "%s", "DB": "%s"},
  "graph": "g",
  "max shift": 2
}`, tmpdir, dbName)
			So(cfg.InitMessage(testutil.JSON(confJSON)), ShouldBeNil)
			var ac AutoCorrelation
			So(ac.Run(ctx, &cfg), ShouldBeNil)
			So(len(g.Plots), ShouldEqual, 1)
			So(len(g.Plots[0].X), ShouldEqual, 2)
		})

	})
}
