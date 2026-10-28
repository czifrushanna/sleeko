// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package eval

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/czifrushanna/sleeko/pkg/baseimage"
)

// RecordedTables replays the base image tables an earlier run offered, so
// a later pipeline faces exactly the same choice: the rule-based baseline
// gets W2's tables, not ones measured days later. It needs no registry or
// scanner, so it cannot be skewed by a rate limit while measuring.
type RecordedTables map[string]baseimage.Table

// LoadRecordedTables reads the tables from records, keyed by their current
// base image. Two records with the same base must hold the same table:
// otherwise which one to replay would be a guess.
func LoadRecordedTables(records []Record) (RecordedTables, error) {
	t := RecordedTables{}
	for _, r := range records {
		if r.BaseImage == nil {
			continue
		}
		ref := r.BaseImage.Table.Current.Ref
		if prev, ok := t[ref]; ok {
			a, _ := json.Marshal(prev)
			b, _ := json.Marshal(r.BaseImage.Table)
			if string(a) != string(b) {
				return nil, fmt.Errorf("the records hold different tables for %s", ref)
			}
			continue
		}
		t[ref] = r.BaseImage.Table
	}
	return t, nil
}

// Propose returns the recorded table for current. A base the earlier run
// offered no table for gets none now either.
func (t RecordedTables) Propose(_ context.Context, current string) (baseimage.Table, error) {
	table, ok := t[current]
	if !ok {
		return baseimage.Table{}, fmt.Errorf("the replayed run offered no table for %s", current)
	}
	return table, nil
}
