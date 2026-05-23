// Package evolve contains the evolutionary loop: fitness scoring, mutation,
// and per-generation orchestration. This file implements the result-line
// parser used in M2 to extract a single JSON object from an attacker pod's
// stdout.
package evolve

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// ErrNoJSONLine is returned by ParseLastJSONLine when no candidate JSON object
// line can be found in the input.
var ErrNoJSONLine = errors.New("evolve: no JSON object line in input")

// ParseLastJSONLine scans the input, splits it into lines, and returns the
// last non-empty line that successfully parses as a single JSON object.
//
// Per the attacker_result_jsonl contract in PRD.json: the pod's final
// non-empty stdout line MUST be a single-line JSON object. To be tolerant of
// hostile pod output (extra logs after the result, missing trailing newline,
// truncated lines) we scan from the end and return the first parseable JSON
// object we find. Lines that look like JSON but fail to parse are skipped;
// non-JSON lines (free-form log output) are skipped.
//
// The returned value is the raw line bytes (without the trailing newline) and
// the decoded map. Callers that need typed access can re-unmarshal into the
// concrete result struct.
func ParseLastJSONLine(r io.Reader) ([]byte, map[string]any, error) {
	sc := bufio.NewScanner(r)
	// Allow large lines: PI agents can emit chunky JSON.
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var lines [][]byte
	for sc.Scan() {
		// Copy because Scanner reuses its underlying buffer between calls.
		b := append([]byte(nil), sc.Bytes()...)
		lines = append(lines, b)
	}
	if err := sc.Err(); err != nil {
		return nil, nil, err
	}

	for i := len(lines) - 1; i >= 0; i-- {
		line := bytesTrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}
		// Fast path: only attempt to parse lines that look like a JSON
		// object. This avoids burning the JSON decoder on every free-form
		// log line.
		if line[0] != '{' || line[len(line)-1] != '}' {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(line, &obj); err != nil {
			continue
		}
		return line, obj, nil
	}
	return nil, nil, ErrNoJSONLine
}

// bytesTrimSpace trims ASCII whitespace from both ends of b. We avoid
// strings.TrimSpace on a []byte->string conversion to keep this allocation-
// light when scanning long pod logs.
func bytesTrimSpace(b []byte) []byte {
	start := 0
	for start < len(b) && isASCIISpace(b[start]) {
		start++
	}
	end := len(b)
	for end > start && isASCIISpace(b[end-1]) {
		end--
	}
	return b[start:end]
}

func isASCIISpace(c byte) bool {
	switch c {
	case ' ', '\t', '\r', '\n', '\v', '\f':
		return true
	}
	return false
}

// ParseLastJSONLineString is a convenience wrapper for callers that already
// hold a string (e.g. after slurping a small log buffer).
func ParseLastJSONLineString(s string) ([]byte, map[string]any, error) {
	return ParseLastJSONLine(strings.NewReader(s))
}
