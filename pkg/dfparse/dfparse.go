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

// Package dfparse is the one place in sleeko that understands Dockerfiles. The
// evaluation metric and the candidate checks both ask it the same questions —
// which base images, which build-context files, which RUN commands, which
// pinned packages, which runtime contract — so they agree on the answers.
//
// Parsing is BuildKit's own (frontend/dockerfile/parser plus instructions), so
// heredocs, line continuations, escape directives and flag validation all come
// from the real builder rather than a re-implementation. A Dockerfile that
// parses here can still fail checks BuildKit only makes while building, such
// as a COPY --from naming a stage that does not exist.
package dfparse

import (
	"errors"
	"fmt"
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/moby/buildkit/frontend/dockerfile/parser"
)

// File is a parsed Dockerfile.
type File struct {
	// MetaArgs are the ARG instructions before the first FROM: the only
	// variables a FROM line can reference.
	MetaArgs []Instruction
	// Stages are the build stages in source order.
	Stages []Stage
	// EscapeToken is the escape character in effect (`\` unless an escape
	// directive changes it).
	EscapeToken rune
}

// Stage is one FROM and the instructions under it.
type Stage struct {
	// Index is the stage's position, from 0.
	Index int
	// Name is the stage name from "AS <name>", lower-cased as BuildKit does;
	// empty for an unnamed stage.
	Name string
	// BaseName is the FROM argument as written, before any ARG expansion.
	BaseName string
	// Platform is the --platform flag as written, if any.
	Platform string
	// From is the FROM instruction itself.
	From Instruction
	// Instructions are the stage's instructions after FROM.
	Instructions []Instruction
}

// StartLine is the line of the stage's FROM.
func (s *Stage) StartLine() int { return s.From.StartLine }

// EndLine is the last line belonging to the stage.
func (s *Stage) EndLine() int {
	if n := len(s.Instructions); n > 0 {
		return s.Instructions[n-1].EndLine
	}
	return s.From.EndLine
}

// Instruction is one Dockerfile instruction with its location.
type Instruction struct {
	// Name is the instruction keyword, lower-cased ("run", "copy", ...).
	Name string
	// Original is the instruction as written, with line continuations joined.
	// Heredoc bodies are not included.
	Original string
	// StartLine and EndLine are 1-based and inclusive; a heredoc's body counts
	// towards the instruction that opens it.
	StartLine int
	EndLine   int

	cmd any
}

// ParseError is a Dockerfile BuildKit would refuse. Line is 0 when the error
// does not belong to one instruction (an empty file, a file with no FROM).
type ParseError struct {
	Line int
	Err  error
}

func (e *ParseError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d: %v", e.Line, e.Err)
	}
	return e.Err.Error()
}

func (e *ParseError) Unwrap() error { return e.Err }

// Parse parses a Dockerfile. Any error is a *ParseError: a syntax error, an
// unknown instruction, an invalid flag, or an instruction other than ARG
// before the first FROM.
func Parse(src string) (*File, error) {
	res, err := parser.Parse(strings.NewReader(src))
	if err != nil {
		return nil, &ParseError{Line: errorLine(err), Err: err}
	}

	f := &File{EscapeToken: res.EscapeToken}
	for _, node := range res.AST.Children {
		cmd, err := instructions.ParseInstruction(node)
		if err != nil {
			return nil, &ParseError{Line: node.StartLine, Err: err}
		}

		in := Instruction{
			Name:      strings.ToLower(node.Value),
			Original:  node.Original,
			StartLine: node.StartLine,
			EndLine:   node.EndLine,
			cmd:       cmd,
		}

		switch c := cmd.(type) {
		case *instructions.Stage:
			f.Stages = append(f.Stages, Stage{
				Index:    len(f.Stages),
				Name:     c.Name,
				BaseName: c.BaseName,
				Platform: c.Platform,
				From:     in,
			})
		case *instructions.ArgCommand:
			if len(f.Stages) == 0 {
				f.MetaArgs = append(f.MetaArgs, in)
				continue
			}
			f.lastStage().Instructions = append(f.lastStage().Instructions, in)
		default:
			if len(f.Stages) == 0 {
				return nil, &ParseError{Line: node.StartLine, Err: fmt.Errorf("%s before the first FROM", strings.ToUpper(in.Name))}
			}
			f.lastStage().Instructions = append(f.lastStage().Instructions, in)
		}
	}

	if len(f.Stages) == 0 {
		return nil, &ParseError{Err: errors.New("no FROM instruction")}
	}
	return f, nil
}

func (f *File) lastStage() *Stage {
	return &f.Stages[len(f.Stages)-1]
}

// FinalStage is the stage a plain `docker build` (no --target) produces.
func (f *File) FinalStage() *Stage {
	return f.lastStage()
}

// stageByName returns the index of the stage a FROM refers to by name,
// considering only stages before `before`, or -1. Names compare
// case-insensitively, as in BuildKit.
func (f *File) stageByName(name string, before int) int {
	name = strings.ToLower(name)
	for i := before - 1; i >= 0; i-- {
		if f.Stages[i].Name != "" && f.Stages[i].Name == name {
			return i
		}
	}
	return -1
}

func errorLine(err error) int {
	var loc *parser.LocationError
	if errors.As(err, &loc) && len(loc.Locations) > 0 && len(loc.Locations[0]) > 0 {
		return loc.Locations[0][0].Start.Line
	}
	return 0
}
