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

package dockerfile

import _ "embed"

// DefaultPrompt is the instruction text prepended to the Dockerfile when a
// deployment does not supply its own via OPTIMIZER_PROMPT /
// sleeko.optimizerPrompt. It matches the exact format pkg/scanreport renders
// the condensed vulnerability summary in, and guards against the pipeline's
// known failure modes (hallucinated fix versions, invented COPY paths the
// build context does not have, deleting the odd RUN line of a
// history-reconstructed Dockerfile).
//
//go:embed prompt.txt
var DefaultPrompt string
