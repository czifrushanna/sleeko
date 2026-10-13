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

package dfparse

import (
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/instructions"
)

// ContextCopy is a COPY or ADD that reads from the build context.
type ContextCopy struct {
	// Stage is the index of the stage the instruction is in.
	Stage int
	// Line is the instruction's first line.
	Line int
	// Instruction is "copy" or "add".
	Instruction string
	// Sources are the context source paths, as written (no ARG/ENV expansion).
	Sources []string
	Dest    string
}

// ContextCopies returns every COPY and ADD that reads from the build context.
// Left out: COPY --from (another stage or an image), heredoc sources (inline
// content), and ADD sources that are URLs or git remotes. An instruction with
// nothing left is left out entirely.
func (f *File) ContextCopies() []ContextCopy {
	var out []ContextCopy
	for _, st := range f.Stages {
		for _, in := range st.Instructions {
			var sources []string
			var dest string
			switch c := in.cmd.(type) {
			case *instructions.CopyCommand:
				if c.From != "" {
					continue
				}
				sources, dest = c.SourcePaths, c.DestPath
			case *instructions.AddCommand:
				for _, p := range c.SourcePaths {
					if !isRemote(p) {
						sources = append(sources, p)
					}
				}
				dest = c.DestPath
			default:
				continue
			}

			if len(sources) == 0 {
				continue
			}
			out = append(out, ContextCopy{
				Stage:       st.Index,
				Line:        in.StartLine,
				Instruction: in.Name,
				Sources:     sources,
				Dest:        dest,
			})
		}
	}
	return out
}

func isRemote(src string) bool {
	return strings.Contains(src, "://") || strings.HasPrefix(src, "git@")
}
