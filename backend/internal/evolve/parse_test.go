package evolve

import (
	"errors"
	"strings"
	"testing"
)

func TestParseLastJSONLine_HappyPath(t *testing.T) {
	in := strings.Join([]string{
		"log1",
		"log2",
		`{"type":"result","status":"BREACH","fitness":1.0}`,
	}, "\n")
	line, obj, err := ParseLastJSONLineString(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(line) != `{"type":"result","status":"BREACH","fitness":1.0}` {
		t.Fatalf("line = %q", line)
	}
	if obj["status"] != "BREACH" {
		t.Fatalf("status = %v", obj["status"])
	}
}

func TestParseLastJSONLine_NoTrailingNewline(t *testing.T) {
	in := "log1\n" + `{"type":"result","status":"NOOP"}`
	_, obj, err := ParseLastJSONLineString(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obj["status"] != "NOOP" {
		t.Fatalf("status = %v", obj["status"])
	}
}

func TestParseLastJSONLine_MultipleJSONLinesPicksLast(t *testing.T) {
	in := strings.Join([]string{
		`{"type":"log","msg":"started"}`,
		`{"type":"log","msg":"fetched"}`,
		`{"type":"result","status":"PARTIAL"}`,
	}, "\n")
	_, obj, err := ParseLastJSONLineString(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obj["status"] != "PARTIAL" {
		t.Fatalf("expected the final result line, got %v", obj)
	}
}

func TestParseLastJSONLine_TrailingGarbageSkipped(t *testing.T) {
	// Free-form garbage after the result line must not poison parsing;
	// the parser should walk back to the most recent valid JSON object.
	in := strings.Join([]string{
		`{"type":"result","status":"BREACH","fitness":1.0}`,
		"pod stopped",
		"",
		"   ",
	}, "\n")
	_, obj, err := ParseLastJSONLineString(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obj["status"] != "BREACH" {
		t.Fatalf("status = %v", obj["status"])
	}
}

func TestParseLastJSONLine_BrokenJSONLineSkipped(t *testing.T) {
	in := strings.Join([]string{
		`{"type":"result","status":"RECON"}`,
		`{this is not json}`,
	}, "\n")
	_, obj, err := ParseLastJSONLineString(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obj["status"] != "RECON" {
		t.Fatalf("status = %v", obj["status"])
	}
}

func TestParseLastJSONLine_NoJSONReturnsSentinel(t *testing.T) {
	in := "just\nlog\nlines\n"
	_, _, err := ParseLastJSONLineString(in)
	if !errors.Is(err, ErrNoJSONLine) {
		t.Fatalf("expected ErrNoJSONLine, got %v", err)
	}
}

func TestParseLastJSONLine_EmptyInput(t *testing.T) {
	_, _, err := ParseLastJSONLineString("")
	if !errors.Is(err, ErrNoJSONLine) {
		t.Fatalf("expected ErrNoJSONLine, got %v", err)
	}
}

func TestParseLastJSONLine_WhitespacePadded(t *testing.T) {
	in := "   \n" + `   {"type":"result","status":"ERROR"}   ` + "\n"
	_, obj, err := ParseLastJSONLineString(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obj["status"] != "ERROR" {
		t.Fatalf("status = %v", obj["status"])
	}
}

func TestParseLastJSONLine_JSONArrayLineRejected(t *testing.T) {
	// The contract specifies a JSON object as the final line. A bare
	// array is not a valid result. We expect ErrNoJSONLine.
	in := `[1,2,3]`
	_, _, err := ParseLastJSONLineString(in)
	if !errors.Is(err, ErrNoJSONLine) {
		t.Fatalf("expected ErrNoJSONLine for bare array, got %v", err)
	}
}
