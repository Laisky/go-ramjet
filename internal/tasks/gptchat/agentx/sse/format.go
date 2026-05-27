// Package sse encodes typed session events onto the existing gptchat SSE
// wire format. See proposal §4.5 for the seamless-integration contract.
//
// The package never imports internal/tasks/gptchat/http directly; the
// handler in 1B-4 supplies an EmitFunc adapter, keeping this package
// testable in isolation without a gin context.
package sse

import "strings"

// toolStepMarker is the per-line prefix that the existing proxy tool loop
// uses for trace lines streamed via emitThinkingDelta. We re-use it
// verbatim so frontend renderers cannot tell agent traces apart from
// proxy traces.
const toolStepMarker = "[[TOOLS]] "

// shortCallIDLen is the prefix length for the per-line call_id prefix
// that disambiguates parallel tool calls in the trace, per §3.8
// invariant 5.
const shortCallIDLen = 6

// short returns the first six characters of a (ULID-shaped) event ID.
// Falls back to the full id if it is shorter than six characters. Used
// to disambiguate parallel tool calls per §3.8 invariant 5.
func short(id string) string {
	if len(id) <= shortCallIDLen {
		return id
	}
	return id[:shortCallIDLen]
}

// chunkString splits s into substrings of at most n bytes. Used to fan
// out Final.Text into bite-sized delta.content chunks. When n is zero
// or negative the input is returned as a single-element slice.
func chunkString(s string, n int) []string {
	if n <= 0 {
		return []string{s}
	}
	if s == "" {
		return nil
	}
	out := make([]string, 0, (len(s)/n)+1)
	for len(s) > 0 {
		if len(s) <= n {
			out = append(out, s)
			break
		}
		out = append(out, s[:n])
		s = s[n:]
	}
	return out
}

// untrustedDelimiterReplacement is the sanitized marker substituted for
// any literal `</tool_result>` substring that a tool returns inside its
// output. Without this guard, a malicious or accidental tool payload
// could prematurely close the `<tool_result trust="untrusted">…</tool_result>`
// envelope established by the system prompt and smuggle text out of the
// untrusted region.
const untrustedDelimiterReplacement = "</tool_result-escaped>"

// escapeUntrustedDelimiter replaces literal `</tool_result>` with a
// sanitized marker so any tool that returns text containing the close
// tag cannot break the system-prompt guard. The wrap hook in
// agentx/loop is expected to perform the same escape upstream of the
// event emit; the function lives here as well because the sse layer
// renders ToolResult content into the trace and must preserve the
// invariant even if a future caller bypasses the wrap hook.
func escapeUntrustedDelimiter(s string) string {
	if s == "" {
		return s
	}
	return strings.ReplaceAll(s, "</tool_result>", untrustedDelimiterReplacement)
}
