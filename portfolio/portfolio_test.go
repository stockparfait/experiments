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

package portfolio

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/stockparfait/db"
	"github.com/stockparfait/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPortfolio(t *testing.T) {
	t.Parallel()

	tmpdir, tmpdirErr := os.MkdirTemp("", "test_portfolio")
	defer os.RemoveAll(tmpdir)

	Convey("Test setup succeeded", t, func() {
		So(tmpdirErr, ShouldBeNil)
	})

	Convey("Portfolio experiment works", t, func() {
		ctx := context.Background()

		dbName := "db"
		csvFile := filepath.Join(tmpdir, "portfolio.csv")
		tickers := map[string]db.TickerRow{
			"A": {
				Exchange: "Exchange A",
				Name:     "Company A",
				Category: "Category A",
				Sector:   "Sector A",
				Industry: "Industry A",
			},
			"B": {
				Exchange: "Exchange B",
				Name:     "Company B",
				Category: "Category B",
				Sector:   "Sector B",
				Industry: "Industry B",
			},
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

		Convey("Two tickers, one explicit cost basis, one derived", func() {
			var cfg config.Portfolio
			So(cfg.InitMessage(testutil.JSON(fmt.Sprintf(`{
  "id": "test",
  "data": {"DB path": "%s", "DB": "%s"},
  "file": "%s",
  "positions": [
    {"ticker": "A", "purchase date": "2019-01-01", "shares": 10, "cost basis": 99},
    {"ticker": "B", "purchase date": "2019-01-01", "shares": 2}
  ],
  "columns": [
    {"kind": "ticker"},
    {"kind": "name"},
    {"kind": "exchange"},
    {"kind": "category"},
    {"kind": "sector"},
    {"kind": "industry"},
    {"kind": "purchase date"},
    {"kind": "cost basis"},
    {"kind": "shares"},
    {"kind": "price", "date": "2019-01-03"},
    {"kind": "value", "date": "2019-01-03"}
  ]
}`, tmpdir, dbName, csvFile))), ShouldBeNil)
			var pe Portfolio
			So(pe.Run(ctx, &cfg), ShouldBeNil)

			f, err := os.Open(csvFile)
			So(err, ShouldBeNil)
			defer f.Close()

			r := csv.NewReader(f)
			csvRows, err := r.ReadAll()
			So(err, ShouldBeNil)
			So(csvRows, ShouldResemble, [][]string{
				{"ticker", "name", "exchange", "category", "sector",
					"industry", "purchase date", "cost basis", "shares",
					"price 2019-01-03", "value 2019-01-03"},
				{"A", "Company A", "Exchange A", "Category A", "Sector A",
					"Industry A", "2019-01-01", "99.00", "10", "11.00", "110.00"},
				{"B", "Company B", "Exchange B", "Category B", "Sector B",
					"Industry B", "2019-01-01", "200.00", "2", "110.00", "220.00"},
			})
		})
	})
}
