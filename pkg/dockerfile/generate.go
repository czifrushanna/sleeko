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

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// generatedDockerfileHeader is prepended to every reconstructed Dockerfile so
// consumers (LLM optimizer, portal, reviewers) can immediately tell it apart
// from a Dockerfile extracted verbatim from BuildKit provenance.
const generatedDockerfileHeader = `# Reconstructed from the image's config history because no BuildKit provenance
# attestation was found on this artifact. This is a best-effort approximation of
# the original build source, not the original Dockerfile: instruction ordering
# and RUN commands are recovered from the image config, but ADD/COPY sources
# cannot be recovered exactly. Review before relying on it.
`

// shellExecPrefixes strips the shell wrapper Docker/BuildKit put around every
// RUN instruction's created_by string before it is turned back into a plain
// "RUN <command>" line.
var shellExecPrefixes = []string{
	"/bin/sh -c ",
	"/bin/bash -c ",
	"cmd /S /C ",
	"powershell -Command ",
}

// addCopyPattern matches the synthetic "#(nop) ADD/COPY [--chown=...] <kind>:<digest> in <dest>"
// form Docker writes into history for ADD/COPY instructions. The original
// source path isn't preserved anywhere in image config, only a digest of the
// content that got added, so it can't be recovered exactly.
var addCopyPattern = regexp.MustCompile(`(?s)^(ADD|COPY)((?:\s+--\S+)*)\s+(?:file|dir|multi):([0-9a-fA-F]+)\s+in\s+(.+?)\s*$`)

// buildkitSuffix marks the history entries BuildKit writes for instructions
// that produce a layer (RUN, COPY, ADD).
const buildkitSuffix = " # buildkit"

// buildkitInstructions are the instruction keywords BuildKit writes at the
// start of a history entry. The classic builder instead writes
// "/bin/sh -c <command>" for RUN and "/bin/sh -c #(nop) <instruction>" for
// everything else.
var buildkitInstructions = map[string]bool{
	"ADD": true, "ARG": true, "CMD": true, "COPY": true, "ENTRYPOINT": true, "ENV": true,
	"EXPOSE": true, "HEALTHCHECK": true, "LABEL": true, "MAINTAINER": true, "ONBUILD": true,
	"RUN": true, "SHELL": true, "STOPSIGNAL": true, "USER": true, "VOLUME": true, "WORKDIR": true,
}

// buildArgPrefix matches the "|<n> " both builders put in front of a RUN that
// had build args in scope; the n KEY=value words that follow it are the args,
// not part of the command.
var buildArgPrefix = regexp.MustCompile(`^\|(\d+)\s+`)

// keyValueStart matches the start of a new KEY=value pair in an ENV or LABEL
// history entry; words that do not match continue the previous value.
var keyValueStart = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_./-]*=`)

// exposedPort matches one port in any of the forms EXPOSE history takes
// ("80/tcp", "[80/tcp 443/tcp]", "map[80/tcp:{}]").
var exposedPort = regexp.MustCompile(`\d+(?:-\d+)?(?:/[a-z]+)?`)

type ociContainerConfig struct {
	Env          []string          `json:"Env"`
	Entrypoint   []string          `json:"Entrypoint"`
	Cmd          []string          `json:"Cmd"`
	WorkingDir   string            `json:"WorkingDir"`
	User         string            `json:"User"`
	ExposedPorts map[string]any    `json:"ExposedPorts"`
	Volumes      map[string]any    `json:"Volumes"`
	Labels       map[string]string `json:"Labels"`
	StopSignal   string            `json:"StopSignal"`
	Healthcheck  *ociHealthcheck   `json:"Healthcheck"`
}

// ociHealthcheck is the image config's healthcheck. Durations are nanoseconds.
type ociHealthcheck struct {
	Test          []string `json:"Test"`
	Interval      int64    `json:"Interval"`
	Timeout       int64    `json:"Timeout"`
	StartPeriod   int64    `json:"StartPeriod"`
	StartInterval int64    `json:"StartInterval"`
	Retries       int      `json:"Retries"`
}

type ociHistoryEntry struct {
	CreatedBy  string `json:"created_by"`
	EmptyLayer bool   `json:"empty_layer"`
}

type ociImageConfig struct {
	Config  ociContainerConfig `json:"config"`
	History []ociHistoryEntry  `json:"history"`
}

// manifestProbe is used both for the top-level index/manifest and for a
// resolved image manifest: it recognizes a plain image manifest via the
// "config" digest field, and a manifest list via "manifests".
type manifestProbe struct {
	Config *struct {
		Digest string `json:"digest"`
	} `json:"config"`
	Manifests []struct {
		Digest      string            `json:"digest"`
		Annotations map[string]string `json:"annotations"`
	} `json:"manifests"`
}

// GenerateDockerfileFromSource reconstructs a best-effort Dockerfile from the
// image config's `history` metadata, which every OCI/Docker image carries
// regardless of how it was built. It is the fallback used when
// ExtractDockerfileFromSource cannot find a BuildKit provenance attestation
// (e.g. images built with the classic builder, or pulled/re-pushed without
// provenance): unlike the provenance path, this never recovers the exact
// original Dockerfile, only an approximation good enough for the LLM
// optimizer and human review.
func GenerateDockerfileFromSource(ctx context.Context, src BlobSource) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	configDigest, err := resolveImageConfigDigest(ctx, src)
	if err != nil {
		return nil, err
	}

	configBytes, err := src.Blob(ctx, configDigest)
	if err != nil {
		return nil, fmt.Errorf("pull image config %s: %w", configDigest, err)
	}

	var cfg ociImageConfig
	if err := json.Unmarshal(configBytes, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal image config %s: %w", configDigest, err)
	}

	if len(cfg.History) == 0 {
		return nil, fmt.Errorf("image config %s has no history metadata to reconstruct a Dockerfile from", configDigest)
	}

	return &Result{
		Dockerfile: renderGeneratedDockerfile(&cfg),
		Generated:  true,
	}, nil
}

// resolveImageConfigDigest finds the digest of the image config blob starting
// from the same top-level reference ExtractDockerfileFromSource looks at. It
// handles both a direct image manifest (config digest at the top level) and a
// manifest list (walking non-attestation entries looking for one).
func resolveImageConfigDigest(ctx context.Context, src BlobSource) (string, error) {
	top, err := src.Index(ctx)
	if err != nil {
		return "", err
	}

	var probe manifestProbe
	if err := json.Unmarshal(top, &probe); err != nil {
		return "", fmt.Errorf("unmarshal manifest: %w", err)
	}

	if probe.Config != nil && probe.Config.Digest != "" {
		return probe.Config.Digest, nil
	}

	for _, m := range probe.Manifests {
		if m.Annotations["vnd.docker.reference.type"] == attestationManifestType {
			continue
		}

		blob, err := src.Blob(ctx, m.Digest)
		if err != nil {
			continue
		}

		var nested manifestProbe
		if err := json.Unmarshal(blob, &nested); err != nil {
			continue
		}
		if nested.Config != nil && nested.Config.Digest != "" {
			return nested.Config.Digest, nil
		}
	}

	return "", fmt.Errorf("no image manifest found to generate a Dockerfile from")
}

func renderGeneratedDockerfile(cfg *ociImageConfig) string {
	var b strings.Builder
	b.WriteString(generatedDockerfileHeader)
	b.WriteString("\nFROM scratch\n")

	seen := map[string]bool{}
	for _, h := range cfg.History {
		line := renderHistoryLine(h.CreatedBy, &cfg.Config)
		if line == "" {
			continue
		}
		b.WriteString(line)
		b.WriteString("\n")
		seen[firstToken(line)] = true
	}

	for _, line := range trailingConfigLines(&cfg.Config, seen) {
		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

// renderHistoryLine turns one image config history entry's created_by string
// into a Dockerfile line. Two builders write history, in different forms:
//
//   - the classic builder: "/bin/sh -c <command>" for RUN and
//     "/bin/sh -c #(nop) <instruction>" for everything else;
//   - BuildKit: the instruction itself ("RUN /bin/sh -c <command>",
//     "WORKDIR /app", "COPY src /dst"), with " # buildkit" appended to the
//     ones that produced a layer.
//
// Both write some arguments in Go's formatting rather than Dockerfile syntax
// (`CMD ["a" "b"]`, `VOLUME [/data]`, `HEALTHCHECK &{...}`), so non-RUN
// instructions go through renderInstruction to come out parseable.
func renderHistoryLine(createdBy string, c *ociContainerConfig) string {
	raw := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(createdBy), buildkitSuffix))
	if raw == "" {
		return ""
	}

	if kw := firstToken(raw); buildkitInstructions[kw] && strings.HasPrefix(raw, kw) {
		if kw == "RUN" {
			return renderRun(strings.TrimSpace(raw[len(kw):]))
		}
		return renderInstruction(raw, c)
	}

	if cmd := stripShellPrefix(raw); strings.HasPrefix(cmd, "#(nop)") {
		instr := strings.TrimSpace(strings.TrimPrefix(cmd, "#(nop)"))
		if instr == "" {
			return ""
		}
		return renderInstruction(instr, c)
	}

	// A bare comment, as Docker's official Debian-based images record their
	// rootfs layer ("# debian.sh --arch 'amd64' out/ 'bookworm' ..."): keep it
	// as a comment rather than a RUN that does nothing.
	if strings.HasPrefix(raw, "#") && !strings.Contains(raw, "\n") {
		return raw
	}

	return renderRun(raw)
}

func stripShellPrefix(s string) string {
	for _, prefix := range shellExecPrefixes {
		if strings.HasPrefix(s, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(s, prefix))
		}
	}
	return s
}

// renderRun turns a recorded command into a RUN line: build args and the shell
// wrapper are dropped, and a command spanning several lines (BuildKit records
// a heredoc RUN's script that way) becomes a heredoc again, since each extra
// line would otherwise be read as an instruction of its own.
func renderRun(cmd string) string {
	if m := buildArgPrefix.FindStringSubmatch(cmd); m != nil {
		// The args are joined with single spaces, so a value that itself
		// contains a space cannot be told apart from the command; such values
		// are rare enough to accept that.
		n, _ := strconv.Atoi(m[1])
		rest := strings.TrimPrefix(cmd, m[0])
		for i := 0; i < n; i++ {
			_, rest, _ = strings.Cut(strings.TrimLeft(rest, " "), " ")
		}
		cmd = rest
	}
	cmd = strings.TrimSpace(stripShellPrefix(strings.TrimSpace(cmd)))
	if cmd == "" {
		return ""
	}

	if !strings.Contains(cmd, "\n") {
		return "RUN " + cmd
	}
	delim := "SLEEKO_EOF"
	for strings.Contains("\n"+cmd+"\n", "\n"+delim+"\n") {
		delim += "_"
	}
	return "RUN <<" + delim + "\n" + cmd + "\n" + delim
}

// renderInstruction turns a recorded non-RUN instruction into valid
// Dockerfile syntax.
func renderInstruction(instr string, c *ociContainerConfig) string {
	kw := firstToken(instr)
	args := strings.TrimSpace(instr[len(kw):])

	switch kw {
	case "ADD", "COPY":
		return rewriteAddCopyPlaceholder(instr)
	case "CMD", "ENTRYPOINT", "SHELL":
		if list, ok := parseRecordedList(args); ok {
			return kw + " " + jsonArray(list)
		}
	case "VOLUME":
		if list, ok := parseRecordedList(args); ok {
			return kw + " " + jsonArray(list)
		}
	case "EXPOSE":
		if ports := exposedPort.FindAllString(args, -1); len(ports) > 0 {
			return kw + " " + strings.Join(ports, " ")
		}
	case "ENV", "LABEL":
		return renderKeyValues(kw, args)
	case "HEALTHCHECK":
		return renderHealthcheck(c.Healthcheck)
	}
	return instr
}

// rewriteAddCopyPlaceholder makes an ADD/COPY whose source cannot be recovered
// parseable: the classic builder records only a digest of the content
// ("COPY file:<hex> in /dst"), and BuildKit records a heredoc source by name
// only ("COPY <<EOT /dst"), its body gone.
func rewriteAddCopyPlaceholder(instr string) string {
	if m := addCopyPattern.FindStringSubmatch(instr); m != nil {
		keyword, flags, digest, dest := m[1], m[2], m[3], m[4]
		return fmt.Sprintf("%s%s <unresolvable-source sha256:%s> %s", keyword, flags, digest, dest)
	}

	words := strings.Fields(instr)
	for i, w := range words {
		if name, ok := strings.CutPrefix(w, "<<"); ok && i > 0 && i < len(words)-1 {
			words[i] = fmt.Sprintf("<unresolvable-source heredoc:%s>", strings.TrimLeft(name, "-"))
		}
	}
	return strings.Join(words, " ")
}

// parseRecordedList reads a list argument the way history records it: as
// JSON, as a Go-quoted list (`["a" "b"]`, which is not JSON once it has two
// elements), or as a bare Go list (`[/data /logs]`).
func parseRecordedList(s string) ([]string, bool) {
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return nil, false
	}

	var list []string
	if err := json.Unmarshal([]byte(s), &list); err == nil {
		return list, true
	}

	inner := strings.TrimSpace(s[1 : len(s)-1])
	if !strings.HasPrefix(inner, `"`) {
		return strings.Fields(inner), true
	}
	for inner != "" {
		lit, err := strconv.QuotedPrefix(inner)
		if err != nil {
			return nil, false
		}
		v, err := strconv.Unquote(lit)
		if err != nil {
			return nil, false
		}
		list = append(list, v)
		inner = strings.TrimSpace(inner[len(lit):])
	}
	return list, true
}

// renderKeyValues re-quotes an ENV or LABEL history entry. History records
// the values unquoted ("LABEL maintainer=NGINX Docker Maintainers <...>"), so
// a value with a space would otherwise read as several malformed pairs.
func renderKeyValues(kw, args string) string {
	var pairs []string
	for _, w := range strings.Fields(args) {
		if keyValueStart.MatchString(w) || len(pairs) == 0 {
			pairs = append(pairs, w)
			continue
		}
		pairs[len(pairs)-1] += " " + w
	}

	for i, p := range pairs {
		key, value, ok := strings.Cut(p, "=")
		if !ok {
			// Not KEY=value at all (an old "ENV KEY value" form): leave it alone.
			return kw + " " + args
		}
		pairs[i] = key + "=" + quoteValue(value)
	}
	return kw + " " + strings.Join(pairs, " ")
}

// quoteValue quotes an ENV/LABEL value when it would not survive as a bare
// word. History holds values after expansion, so "$" is escaped to stay
// literal.
func quoteValue(v string) string {
	if v != "" && !strings.ContainsAny(v, " \t\"'\\$") {
		return v
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`)
	return `"` + r.Replace(v) + `"`
}

// renderHealthcheck renders the image config's healthcheck. Both builders
// record HEALTHCHECK in Go struct syntax ("&{[CMD-SHELL ...] 30s ...}"),
// which cannot be parsed back reliably, while the config holds the same
// settings exactly. With no healthcheck in the config there is nothing to
// render.
func renderHealthcheck(h *ociHealthcheck) string {
	if h == nil || len(h.Test) == 0 {
		return ""
	}
	if h.Test[0] == "NONE" {
		return "HEALTHCHECK NONE"
	}

	var b strings.Builder
	b.WriteString("HEALTHCHECK")
	for _, opt := range []struct {
		flag string
		ns   int64
	}{
		{"--interval", h.Interval},
		{"--timeout", h.Timeout},
		{"--start-period", h.StartPeriod},
		{"--start-interval", h.StartInterval},
	} {
		if opt.ns > 0 {
			fmt.Fprintf(&b, " %s=%s", opt.flag, time.Duration(opt.ns))
		}
	}
	if h.Retries > 0 {
		fmt.Fprintf(&b, " --retries=%d", h.Retries)
	}

	switch h.Test[0] {
	case "CMD-SHELL":
		b.WriteString(" CMD " + strings.Join(h.Test[1:], " "))
	case "CMD":
		b.WriteString(" CMD " + jsonArray(h.Test[1:]))
	default:
		return ""
	}
	return b.String()
}

func firstToken(line string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	return strings.ToUpper(fields[0])
}

// trailingConfigLines is a safety net for images whose history was pruned or
// never carried certain metadata (e.g. re-committed images): it emits, from
// the final config state, only the instruction kinds that history didn't
// already contribute.
func trailingConfigLines(c *ociContainerConfig, seen map[string]bool) []string {
	var lines []string

	if !seen["ENV"] {
		for _, kv := range c.Env {
			lines = append(lines, renderKeyValues("ENV", kv))
		}
	}

	if !seen["LABEL"] && len(c.Labels) > 0 {
		for _, k := range sortedKeys(c.Labels) {
			lines = append(lines, fmt.Sprintf("LABEL %s=%q", k, c.Labels[k]))
		}
	}

	if !seen["EXPOSE"] && len(c.ExposedPorts) > 0 {
		for _, p := range sortedKeys(c.ExposedPorts) {
			lines = append(lines, "EXPOSE "+strings.SplitN(p, "/", 2)[0])
		}
	}

	if !seen["VOLUME"] && len(c.Volumes) > 0 {
		for _, v := range sortedKeys(c.Volumes) {
			lines = append(lines, fmt.Sprintf("VOLUME [%q]", v))
		}
	}

	if !seen["WORKDIR"] && c.WorkingDir != "" {
		lines = append(lines, "WORKDIR "+c.WorkingDir)
	}

	if !seen["USER"] && c.User != "" {
		lines = append(lines, "USER "+c.User)
	}

	if !seen["STOPSIGNAL"] && c.StopSignal != "" {
		lines = append(lines, "STOPSIGNAL "+c.StopSignal)
	}

	if !seen["ENTRYPOINT"] && len(c.Entrypoint) > 0 {
		lines = append(lines, "ENTRYPOINT "+jsonArray(c.Entrypoint))
	}

	if !seen["CMD"] && len(c.Cmd) > 0 {
		lines = append(lines, "CMD "+jsonArray(c.Cmd))
	}

	return lines
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func jsonArray(items []string) string {
	if items == nil {
		items = []string{} // "[]", not "null"
	}
	b, _ := json.Marshal(items)
	return string(b)
}
