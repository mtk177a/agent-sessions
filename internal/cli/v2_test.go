package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/mtk177a/agent-sessions/internal/config"
	"github.com/mtk177a/agent-sessions/internal/contract"
	"github.com/mtk177a/agent-sessions/internal/provider"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRecordedContentSurvivesOutputPreparation(t *testing.T) {
	text := "token=fictional-only /fictional/source host.example.invalid\n$ go test ./..."
	events := []contract.Event{{Kind: contract.EventMessage, Message: &contract.MessageEvent{Role: "user", Text: text}, Metadata: []contract.Metadata{}}}
	envelope := contract.NewEnvelope("events", "test", contract.StatusComplete)
	envelope.Data = &contract.Data{Events: &events}
	if err := prepareEnvelope(&envelope); err != nil {
		t.Fatal(err)
	}
	if events[0].Message.Text != text || envelope.Status != contract.StatusComplete {
		t.Fatal("recorded content was changed or treated as an omission")
	}
}

func TestEventsBoundedContentAndPagination(t *testing.T) {
	root := t.TempDir()
	adapter := newSyntheticAdapter()
	original := strings.Repeat("日", contract.MaxStringBytes)
	adapter.events = []contract.Event{
		{Kind: contract.EventToolCall, ToolCall: &contract.ToolCallEvent{CallID: "call-1", Name: "fictional", Category: "shell", Action: "execute", InputState: contract.InputAvailable, Input: contract.JSONContent([]string{"go", original})}, Metadata: []contract.Metadata{}},
		{Kind: contract.EventToolResult, ToolResult: &contract.ToolResultEvent{CallID: "call-1", Outcome: "unknown", CorrelationState: "matched", ContentState: contract.EvidenceAvailable, Content: contract.TextContent(original)}, Metadata: []contract.Metadata{}},
		{Kind: contract.EventToolResult, ToolResult: &contract.ToolResultEvent{Outcome: "failure", CorrelationState: "unmatched", ContentState: contract.EvidenceAvailable, Content: contract.TextContent("error /fictional/source token=fictional")}, Metadata: []contract.Metadata{}},
	}
	omissions := contract.ToolInputOmissions(adapter.events)
	for _, e := range adapter.events {
		if e.ToolResult != nil {
			omissions = append(omissions, contract.ToolResultOmissions(*e.ToolResult)...)
		}
	}
	runner := Runner{Version: "test", Registry: provider.NewRegistry(&eventResultAdapter{syntheticAdapter: adapter, result: provider.EventResult{Status: contract.StatusPartial, Events: adapter.events, Omissions: omissions}})}
	ref := adapter.source.Identity.SourceRef
	var collected []contract.Event
	cursor := ""
	for {
		args := []string{"events", "--root", root, "--limit", "1", ref}
		if cursor != "" {
			args = []string{"events", "--root", root, "--limit", "1", "--cursor", cursor, ref}
		}
		var output bytes.Buffer
		if exit := runner.Run(t.Context(), args, &output); exit != ExitOK {
			t.Fatalf("exit %d: %s", exit, output.String())
		}
		var envelope contract.Envelope
		if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Status != contract.StatusPartial || envelope.SchemaVersion != "v2" {
			t.Fatal("invalid state")
		}
		collected = append(collected, (*envelope.Data.Events)...)
		if !envelope.Page.HasMore {
			break
		}
		cursor = envelope.Page.NextCursor
	}
	if len(collected) != len(adapter.events) {
		t.Fatal("missing or duplicated events")
	}
	for i, e := range collected {
		if e.Index != uint64(i) {
			t.Fatal("page order changed")
		}
	}
	input := collected[0].ToolCall.Input
	if !input.Truncated || input.Format != "json" || !utf8.ValidString(input.Text) || len(input.Text) > contract.MaxStringBytes || json.Valid([]byte(input.Text)) {
		t.Fatal("incorrect input truncation")
	}
	result := collected[1].ToolResult
	if !result.Content.Truncated || !utf8.ValidString(result.Content.Text) || !strings.HasPrefix(original, result.Content.Text) || result.Outcome != "unknown" || result.CallID != "call-1" {
		t.Fatal("incorrect result truncation or correlation across pages")
	}
	if collected[2].ToolResult.Content.Text != "error /fictional/source token=fictional" || collected[2].ToolResult.CallID != "" {
		t.Fatal("standalone result lost")
	}
}

func TestResponseLimitCanBeRetriedFromSameCursor(t *testing.T) {
	adapter := newSyntheticAdapter()
	adapter.events = nil
	for i := 0; i < 30; i++ {
		adapter.events = append(adapter.events, contract.Event{Kind: contract.EventMessage, Message: &contract.MessageEvent{Role: "assistant", Text: strings.Repeat("\x01", contract.MaxStringBytes)}, Metadata: []contract.Metadata{}})
	}
	runner := Runner{Version: "test", Registry: provider.NewRegistry(adapter)}
	root := t.TempDir()
	ref := adapter.source.Identity.SourceRef
	var output bytes.Buffer
	if exit := runner.Run(t.Context(), []string{"events", "--root", root, "--limit", "30", ref}, &output); exit != ExitFailure {
		t.Fatal("oversized response was accepted")
	}
	var envelope contract.Envelope
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error == nil || envelope.Error.Code != "response_limit_exceeded" || len(output.Bytes()) > MaxResponseBytes {
		t.Fatal("incorrect response limit error")
	}
	output.Reset()
	if exit := runner.Run(t.Context(), []string{"events", "--root", root, "--limit", "1", ref}, &output); exit != ExitOK {
		t.Fatal("smaller request failed")
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if (*envelope.Data.Events)[0].Index != 0 || (*envelope.Data.Events)[0].Message.Text != adapter.events[0].Message.Text || !envelope.Page.HasMore {
		t.Fatal("retry skipped or altered first event")
	}
}

type eventResultAdapter struct {
	*syntheticAdapter
	result provider.EventResult
}

func (a *eventResultAdapter) Events(context.Context, config.Source, string) provider.EventResult {
	return a.result
}
