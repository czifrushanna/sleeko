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
	"fmt"
	"sort"
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/moby/buildkit/frontend/dockerfile/shell"
)

// FromRef is what one stage builds on.
type FromRef struct {
	// Stage is the index of the stage this FROM opens.
	Stage int
	// Line is the line of the FROM instruction.
	Line int
	// Raw is the FROM argument as written.
	Raw string
	// Image is Raw with the meta ARGs expanded. For a stage reference it is the
	// referenced stage's name.
	Image string
	// BaseStage is the index of the earlier stage this FROM builds on, or -1
	// when it names an image.
	BaseStage int
	// Scratch reports FROM scratch, which has no base image at all.
	Scratch bool
	// Unresolved lists the variables Raw uses that have no value: a meta ARG
	// without a default and no build arg, or a variable that was never declared
	// (BuildKit's automatic platform ARGs among them). Image is only a
	// trustworthy reference when this is empty.
	Unresolved []string
}

// IsImage reports whether the stage builds on a registry image, as opposed to
// an earlier stage or scratch.
func (r FromRef) IsImage() bool {
	return r.BaseStage < 0 && !r.Scratch
}

// FromRefs returns every stage's base, resolving meta ARGs the way BuildKit
// does for FROM: an ARG before the first FROM takes its value from buildArgs
// when present, else from its default, which may itself use earlier meta
// ARGs. buildArgs may be nil.
func (f *File) FromRefs(buildArgs map[string]string) ([]FromRef, error) {
	lex := shell.NewLex(f.EscapeToken)

	env := envMap{}
	for _, in := range f.MetaArgs {
		arg := in.cmd.(*instructions.ArgCommand)
		for _, kv := range arg.Args {
			if v, ok := buildArgs[kv.Key]; ok {
				env[kv.Key] = v
				continue
			}
			if kv.Value == nil {
				continue
			}
			v, _, err := lex.ProcessWord(*kv.Value, env)
			if err != nil {
				return nil, &ParseError{Line: in.StartLine, Err: fmt.Errorf("expand ARG %s: %w", kv.Key, err)}
			}
			env[kv.Key] = v
		}
	}

	refs := make([]FromRef, 0, len(f.Stages))
	for i := range f.Stages {
		st := &f.Stages[i]
		res, err := lex.ProcessWordWithMatches(st.BaseName, env)
		if err != nil {
			return nil, &ParseError{Line: st.From.StartLine, Err: fmt.Errorf("expand FROM %s: %w", st.BaseName, err)}
		}

		ref := FromRef{
			Stage:      i,
			Line:       st.From.StartLine,
			Raw:        st.BaseName,
			Image:      res.Result,
			BaseStage:  f.stageByName(res.Result, i),
			Unresolved: sortedSet(res.Unmatched),
		}
		ref.Scratch = ref.BaseStage < 0 && strings.EqualFold(ref.Image, "scratch")
		refs = append(refs, ref)
	}
	return refs, nil
}

// envMap is the shell.EnvGetter for meta ARG expansion.
type envMap map[string]string

func (e envMap) Get(k string) (string, bool) {
	v, ok := e[k]
	return v, ok
}

func (e envMap) Keys() []string {
	return sortedSet(e)
}

func sortedSet[V any](m map[string]V) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
