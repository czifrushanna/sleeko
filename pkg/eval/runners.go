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

	"github.com/czifrushanna/sleeko/pkg/baseimage"
	"github.com/czifrushanna/sleeko/pkg/pipeline"
	"github.com/czifrushanna/sleeko/pkg/score"
	"github.com/czifrushanna/sleeko/pkg/verify"
)

// Source checks out an item's repository at its commit.
type Source interface {
	// Checkout returns the directory holding the repository at the commit.
	// The directory is shared and must be treated as read-only.
	Checkout(ctx context.Context, repo, commit string) (string, error)
}

// BuildSpec is one image build. O′ and C share everything but Dockerfile.
type BuildSpec struct {
	// ContextDir is the build context directory.
	ContextDir string
	// Dockerfile is the path of the Dockerfile to build.
	Dockerfile string
	BuildArgs  map[string]string
	Platform   string
	Target     string
	// Tag names the resulting image so it can be run.
	Tag string
	// OutDir is where the builder may write the image archive.
	OutDir string
}

// Image is a built image.
type Image struct {
	Tag string
	// Digest is the image manifest digest.
	Digest string
	// Size is the compressed size: the sum of the manifest's layer sizes.
	Size int64
	// Contract is the runtime contract from the image config, for G5.
	Contract score.Contract
	// Archive is the image archive the scanner reads.
	Archive string
}

// Builder builds images.
type Builder interface {
	Build(ctx context.Context, spec BuildSpec) (Image, error)
	// Remove deletes what Build left behind for the image.
	Remove(ctx context.Context, img Image) error
}

// ScanOutput is a scan of one image.
type ScanOutput struct {
	Result score.ScanResult
	// OS is the distribution the scanner detected ("debian 12.8"), for
	// checking the manifest's distro family.
	OS string
	// EOSL: the scanner reports OS past its end of life (as of the scan),
	// so findings disclosed since are missing. Nil in scans cached before
	// it was recorded, which are scanned again.
	EOSL *bool
}

// Scanner scans images with a fixed scanner version and database.
type Scanner interface {
	Scan(ctx context.Context, img Image) (ScanOutput, error)
	// DBVersion identifies the vulnerability database in use; it is part of
	// every cache key and every record.
	DBVersion(ctx context.Context) (string, error)
}

// Smoker runs an item's smoke test against an image.
type Smoker interface {
	// Smoke returns nil when the test passes, and an error saying why not
	// otherwise.
	Smoke(ctx context.Context, img Image, platform string, smoke Smoke) error
}

// OptimizeInput is what the pipeline gets: what the adapter gets from Harbor.
type OptimizeInput struct {
	Dockerfile string
	// ScanReport is the Harbor-shape report of O′.
	ScanReport string
	// Repetition is the attempt's repetition number, added to the LLM's
	// base seed so repetitions sample differently.
	Repetition int
}

// OptimizeOutput is one pipeline run's result.
type OptimizeOutput struct {
	Candidate   string
	LLMRounds   int
	Diagnostics []string
	// Verification is the verify stage's rounds, when the pipeline has one.
	Verification []pipeline.VerifyRound
	// BaseTable is the base image table W2 offered, when it offered one.
	BaseTable *baseimage.Table
}

// VersionChecker checks that what a candidate pins exists (G3);
// *verify.Verifier implements it.
type VersionChecker interface {
	Existence(ctx context.Context, in verify.Input) verify.Result
}

// Optimizer runs the pipeline under evaluation.
type Optimizer interface {
	Optimize(ctx context.Context, in OptimizeInput) (OptimizeOutput, error)
}
