#!/usr/bin/env python3
# Copyright Project Harbor Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""Why candidates do not build: G4 failures by cause, per pipeline.

    python3 eval/plots/build_failures.py RESULTS_DIR [--baseline BASELINE_DIR] [OUT.md]

reads the G4_build reasons of the records and sorts each failure into one
cause by fixed rules, the first that matches. The records keep only the tail
of the build log, so for a long install command the error itself may be cut
off; the step that failed is always kept, and a failure is then classified by
that step. Writes OUT.md (default RESULTS_DIR/build-failures.md) with the
counts and, for every failure, its cause and the line it was classified by.
"""

import json
import os
import re
import sys
from collections import Counter

PIPELINES = [("w0", "W0"), ("w1", "W0+W1"), ("w2", "W0+W1+W2")]

# Causes, in the order they are tried and shown.
CAUSES = [
    ("base_image", "Base image does not exist or is malformed"),
    ("structure", "Error in the Dockerfile's structure"),
    ("missing_tool", "Tool missing from the image"),
    ("syspkg_pinned", "Installing pinned system packages"),
    ("syspkg_other", "Other system package failures"),
    ("lang_deps", "Installing language dependencies"),
    ("compile", "Building or compiling the application"),
    ("unclear", "Unclear: mixed step, error cut off"),
]

# The step BuildKit reports as failed: 'process "/bin/sh -c ..." did not complete'.
STEP = re.compile(r'failed to solve: process "(?P<cmd>.*?)" did not complete successfully: exit code: (?P<code>\d+)', re.S)

BASE_IMAGE = re.compile(
    r"failed to resolve source metadata|pull access denied|invalid reference format|invalid checksum digest")
STRUCTURE = re.compile(
    r'target stage "[^"]*" could not be found|failed to calculate checksum of ref|: not a directory')
MISSING_TOOL = re.compile(r"\b(sh|bash): (line \d+: )?[\w./-]+: not found|command not found")
REPO_GONE = re.compile(r"does no longer have a Release file")
SYSPKG_ERROR = re.compile(
    r"E: Version '[^']*' for '[^']*' was not found|unable to select packages|Unable to locate package"
    r"|has no installation candidate|E: Failed to fetch|held broken packages|unmet dependencies"
    r"|No match for argument")
LANG_DEPS_ERROR = re.compile(
    r"ResolutionImpossible|No matching distribution|Could not find a version|npm (ERR!|error)"
    r"|Bundler cannot continue|Error installing|Could not find gem|PluginResolutionException"
    r"|go: [\w./-]+@\S+: (invalid version|unknown revision)|missing go\.sum entry|Failed to parse: `")
COMPILE_ERROR = re.compile(r"error TS\d+|error\[E\d+\]|error CS\d+|\[ERROR\] COMPILATION ERROR")
# A version pin: apt's pkg=1.2-3, apk's pkg=1.2-r0 or pkg>=1.2.
PIN = re.compile(r"(^|\s)[\w.+-]+(=|>=)[\w:.~+-]*\d")

# The kind of each command in a step, by the program it runs.
KINDS = [
    ("syspkg", re.compile(r"^(apt-get|apt|apk|dnf|microdnf|yum)\s+(-\S+\s+)*(install|add|upgrade|update)\b")),
    ("lang_deps", re.compile(
        r"^(pip3?|python3? -m pip|uv pip|npm (ci|install|i|update)|yarn( install)?$|pnpm install|gem install|bundle( install)?\b"
        r"|go (get|mod)|composer (install|update|require)|mix (deps|local)|cargo fetch)")),
    ("compile", re.compile(
        r"^(go build|\./gradlew|gradle|mvn|cargo build|dotnet (publish|build|pack)|npm run|pnpm( run)? build"
        r"|yarn (run )?build|tsc|make|docker-php-ext-install)\b")),
]


def step_kinds(cmd):
    """The kinds of command a failed step runs, ignoring setup like cd or rm."""
    cmd = cmd.replace("\\t", " ").replace("\\n", " ").replace('\\"', '"')
    cmd = re.sub(r"^/bin/(ba)?sh( -o pipefail)? -c ", "", cmd)
    kinds = set()
    for part in re.split(r"&&|\|\||;|\|", cmd):
        part = part.strip()
        part = re.sub(r"^(\w+=\S*\s+)+", "", part)  # VAR=value prefixes
        for kind, rx in KINDS:
            if rx.search(part):
                kinds.add(kind)
    return kinds, cmd


def classify(reason):
    """The cause of one G4 failure, and the line it was decided by."""
    def line_of(rx):
        for line in reason.splitlines():
            if rx.search(line):
                return line.strip()
        return ""

    if BASE_IMAGE.search(reason):
        return "base_image", line_of(BASE_IMAGE)
    if STRUCTURE.search(reason):
        return "structure", line_of(STRUCTURE)
    step = STEP.search(reason)
    raw, code = (step.group("cmd"), step.group("code")) if step else ("", "")
    kinds, cmd = step_kinds(raw)
    if code == "127" or MISSING_TOOL.search(reason):
        return "missing_tool", line_of(MISSING_TOOL) or f"exit code 127: {cmd}"
    if REPO_GONE.search(reason):
        return "syspkg_other", line_of(REPO_GONE)
    if SYSPKG_ERROR.search(reason):
        return ("syspkg_pinned" if PIN.search(cmd) else "syspkg_other"), line_of(SYSPKG_ERROR)
    if LANG_DEPS_ERROR.search(reason):
        return "lang_deps", line_of(LANG_DEPS_ERROR)
    if COMPILE_ERROR.search(reason):
        return "compile", line_of(COMPILE_ERROR)
    # The error itself was cut off: decide by the step that failed, when it
    # runs only one kind of command.
    if len(kinds) == 1:
        kind = kinds.pop()
        if kind == "syspkg":
            kind = "syspkg_pinned" if PIN.search(cmd) else "syspkg_other"
        return kind, f"step: {cmd}"
    return "unclear", (cmd and f"step: {cmd}") or reason.strip().splitlines()[-1]


def failures(path):
    """(record, reason) for every measured attempt that failed G4."""
    with open(path) as f:
        for line in f:
            r = json.loads(line)
            if r.get("status") != "measured":
                continue
            for g in r.get("gates") or []:
                if g["id"] == "G4_build" and g["status"] == "fail":
                    yield r, g.get("reason") or ""


def main():
    args = sys.argv[1:]
    baseline = None
    if "--baseline" in args:
        i = args.index("--baseline")
        baseline = args[i + 1]
        del args[i:i + 2]
    if len(args) not in (1, 2):
        sys.exit(__doc__)
    results = args[0]
    out = args[1] if len(args) == 2 else os.path.join(results, "build-failures.md")

    sources = [(name, os.path.join(results, f"{key}.jsonl")) for key, name in PIPELINES]
    if baseline:
        sources.append(("Baseline", os.path.join(baseline, "baseline.jsonl")))
    counts, rows = {}, []
    for name, path in sources:
        c = Counter()
        for r, reason in failures(path):
            cause, line = classify(reason)
            c[cause] += 1
            rows.append((name, r["item"], r["repetition"], cause, line))
        counts[name] = c

    names = [n for n, _ in sources]
    lines = ["# Why candidates do not build", "",
             "G4 failures by cause, the first rule that matches (eval/plots/build_failures.py).",
             "The records keep the tail of the build log; where the error itself was cut off,",
             "the failure is classified by the step that failed.", "",
             "| Cause | " + " | ".join(names) + " |",
             "|---|" + "---:|" * len(names)]
    for key, label in CAUSES:
        lines.append(f"| {label} | " + " | ".join(str(counts[n][key]) for n in names) + " |")
    lines.append("| **G4 failures** | " + " | ".join(str(sum(counts[n].values())) for n in names) + " |")
    lines += ["", "## Every failure", "", "| Pipeline | Item | Rep | Cause | Classified by |", "|---|---|---:|---|---|"]
    for name, item, rep, cause, line in rows:
        shown = line.replace("|", "\\|").replace("\n", " ")
        if len(shown) > 160:
            shown = shown[:157] + "..."
        lines.append(f"| {name} | {item} | {rep} | {cause} | `{shown}` |")
    with open(out, "w") as f:
        f.write("\n".join(lines) + "\n")
    print(f"wrote {out}")


if __name__ == "__main__":
    main()
