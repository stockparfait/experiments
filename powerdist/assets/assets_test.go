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

package assets

import (
	"testing"

	"github.com/stockparfait/experiments/config"

	. "github.com/smartystreets/goconvey/convey"
)

func TestConfigs(t *testing.T) {
	t.Parallel()

	configs := []string{
		"normal-N-250-mean-mad-sigma.json",
		"normal-N-5K-mean-mad-sigma.json",
		"normal-N-20M-mean-mad-sigma.json",
		"t-N-250-mean-mad-sigma.json",
		"t-N-5K-all-dist.json",
		"t-N-20M-all-dist.json",
		"t-a28-vs-a32-linear.json",
		"t-a28-vs-a32-log.json",
	}

	Convey("Configs parse successfully", t, func() {
		for _, conf := range configs {
			_, err := config.Load(conf)
			So(err, ShouldBeNil)
		}
	})
}
