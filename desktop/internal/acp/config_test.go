package acp

import (
	"context"
	"testing"

	sdk "github.com/caelis-labs/acp-go-sdk"
)

func stringPtr(s string) *string { return &s }

func categoryPtr(c sdk.SessionConfigOptionCategory) *sdk.SessionConfigOptionCategory { return &c }

// fakeConfigOptions is what the fake agent advertises: an ungrouped model
// select, a grouped thinking-level select and a boolean toggle, plus an option
// of a kind this client does not know (it is skipped).
func fakeConfigOptions() []sdk.SessionConfigOption {
	modelChoices := sdk.SessionConfigSelectOptionsUngrouped{
		{Value: "sonnet", Name: "Sonnet", Description: stringPtr("fast")},
		{Value: "opus", Name: "Opus"},
	}
	thinkingChoices := sdk.SessionConfigSelectOptionsGrouped{
		{
			Group: "light", Name: "Light",
			Options: []sdk.SessionConfigSelectOption{
				{Value: "low", Name: "Low"},
			},
		},
		{
			Group: "heavy", Name: "Heavy",
			Options: []sdk.SessionConfigSelectOption{
				{Value: "medium", Name: "Medium", Description: stringPtr("balanced")},
				{Value: "high", Name: "High"},
			},
		},
	}
	return []sdk.SessionConfigOption{
		{Select: &sdk.SessionConfigOptionSelect{
			Id:           "model",
			Name:         "Model",
			Category:     categoryPtr(sdk.SessionConfigOptionCategoryModel),
			CurrentValue: "opus",
			Description:  stringPtr("the model to run"),
			Options:      sdk.SessionConfigSelectOptions{Ungrouped: &modelChoices},
		}},
		{Select: &sdk.SessionConfigOptionSelect{
			Id:           "thinking",
			Name:         "Thinking",
			Category:     categoryPtr(sdk.SessionConfigOptionCategoryThoughtLevel),
			CurrentValue: "high",
			Options:      sdk.SessionConfigSelectOptions{Grouped: &thinkingChoices},
		}},
		{Boolean: &sdk.SessionConfigOptionBoolean{
			Id:           "web_search",
			Name:         "Web search",
			Category:     categoryPtr(sdk.SessionConfigOptionCategoryModelConfig),
			CurrentValue: true,
		}},
	}
}

func TestMapConfigOptions(t *testing.T) {
	// The last option is a kind this client does not know: it is skipped.
	options := mapConfigOptions(append(fakeConfigOptions(), sdk.SessionConfigOption{}))
	if len(options) != 3 {
		t.Fatalf("mapped %d options, want 3: %+v", len(options), options)
	}

	model := options[0]
	if model.ID != "model" || model.Name != "Model" {
		t.Errorf("model option = %+v, want model/Model", model)
	}
	if model.Category != "model" {
		t.Errorf("model category = %q, want model", model.Category)
	}
	if model.Kind != "select" {
		t.Errorf("model kind = %q, want select", model.Kind)
	}
	if model.Current != "opus" {
		t.Errorf("model current = %q, want opus", model.Current)
	}
	wantModelChoices := []ConfigChoice{
		{Value: "sonnet", Name: "Sonnet", Description: "fast"},
		{Value: "opus", Name: "Opus"},
	}
	if !sameChoices(model.Choices, wantModelChoices) {
		t.Errorf("model choices = %+v, want %+v", model.Choices, wantModelChoices)
	}
	if model.Boolean {
		t.Errorf("model boolean = true, want false (a select)")
	}

	thinking := options[1]
	if thinking.ID != "thinking" || thinking.Kind != ConfigKindSelect {
		t.Errorf("thinking option = %+v, want a select named thinking", thinking)
	}
	if thinking.Category != "thought_level" || thinking.Current != "high" {
		t.Errorf("thinking category/current = %q/%q, want thought_level/high", thinking.Category, thinking.Current)
	}
	wantThinkingChoices := []ConfigChoice{
		{Value: "low", Name: "Low"},
		{Value: "medium", Name: "Medium", Description: "balanced"},
		{Value: "high", Name: "High"},
	}
	if !sameChoices(thinking.Choices, wantThinkingChoices) {
		t.Errorf("thinking choices = %+v, want grouped choices flattened in order %+v", thinking.Choices, wantThinkingChoices)
	}

	web := options[2]
	if web.ID != "web_search" || web.Kind != "boolean" {
		t.Errorf("web option = %+v, want a boolean named web_search", web)
	}
	if !web.Boolean {
		t.Errorf("web boolean = false, want true")
	}
	if web.Current != "" {
		t.Errorf("web current = %q, want empty for a boolean", web.Current)
	}
	if web.Choices != nil {
		t.Errorf("web choices = %+v, want nil for a boolean", web.Choices)
	}
	if web.Category != "model_config" {
		t.Errorf("web category = %q, want model_config", web.Category)
	}

	if got := mapConfigOptions(nil); got != nil {
		t.Errorf("mapConfigOptions(nil) = %+v, want nil", got)
	}
}

func TestResourceLinkBlock(t *testing.T) {
	block := ResourceLinkBlock("notes.md", "file:///tmp/notes.md")
	if block.ResourceLink == nil {
		t.Fatalf("block carries no resource link: %+v", block)
	}
	link := block.ResourceLink
	if link.Type != "resource_link" {
		t.Errorf("type = %q, want resource_link", link.Type)
	}
	if link.Name != "notes.md" || link.Uri != "file:///tmp/notes.md" {
		t.Errorf("link = %+v, want notes.md at file:///tmp/notes.md", link)
	}
	if block.Text != nil || block.Image != nil || block.Resource != nil {
		t.Errorf("block carries another kind too: %+v", block)
	}
}

// TestConfigOptionsLoopback drives NewSession and both SetConfigOption
// variants through the SDK over in-memory pipes, so the request shapes and the
// mapped answers are the ones a real agent would see.
func TestConfigOptionsLoopback(t *testing.T) {
	lb := newLoopback(t)
	lb.agent.setSessionConfig(fakeConfigOptions())
	ctx := context.Background()

	session, err := lb.client.NewSession(ctx, "/tmp/workspace")
	if err != nil {
		t.Fatalf("session/new: %v", err)
	}
	if session.SessionID != "sess-1" {
		t.Errorf("session id = %q, want sess-1", session.SessionID)
	}
	if len(session.ConfigOptions) != 3 {
		t.Fatalf("session config options = %d, want 3: %+v", len(session.ConfigOptions), session.ConfigOptions)
	}
	if session.ConfigOptions[0].ID != "model" || session.ConfigOptions[0].Current != "opus" {
		t.Errorf("advertised model option = %+v, want model current opus", session.ConfigOptions[0])
	}
	if session.ConfigOptions[2].Kind != ConfigKindBoolean || !session.ConfigOptions[2].Boolean {
		t.Errorf("advertised boolean option = %+v, want web_search on", session.ConfigOptions[2])
	}

	options, err := lb.client.SetConfigOption(ctx, "sess-1", "model", "sonnet")
	if err != nil {
		t.Fatalf("set config option: %v", err)
	}
	if len(options) != 3 || options[0].Current != "sonnet" {
		t.Errorf("options after select = %+v, want model current sonnet", options)
	}

	options, err = lb.client.SetConfigOptionBool(ctx, "sess-1", "web_search", false)
	if err != nil {
		t.Fatalf("set config option bool: %v", err)
	}
	if options[2].Boolean {
		t.Errorf("boolean after toggle = %+v, want web_search off", options[2])
	}

	requests := lb.agent.configRequestList()
	if len(requests) != 2 {
		t.Fatalf("agent saw %d config requests, want 2", len(requests))
	}
	selectRequest := requests[0]
	if selectRequest.ValueId == nil || selectRequest.Boolean != nil {
		t.Fatalf("select request = %+v, want the valueId variant only", selectRequest)
	}
	if selectRequest.ValueId.SessionId != "sess-1" ||
		selectRequest.ValueId.ConfigId != "model" ||
		selectRequest.ValueId.Value != "sonnet" {
		t.Errorf("select request = %+v, want sess-1/model/sonnet", selectRequest.ValueId)
	}

	booleanRequest := requests[1]
	if booleanRequest.Boolean == nil || booleanRequest.ValueId != nil {
		t.Fatalf("boolean request = %+v, want the boolean variant only", booleanRequest)
	}
	if booleanRequest.Boolean.Type != "boolean" {
		t.Errorf("boolean request type = %q, want boolean", booleanRequest.Boolean.Type)
	}
	if booleanRequest.Boolean.SessionId != "sess-1" ||
		booleanRequest.Boolean.ConfigId != "web_search" ||
		booleanRequest.Boolean.Value {
		t.Errorf("boolean request = %+v, want sess-1/web_search/false", booleanRequest.Boolean)
	}
}

// TestConfigOptionNoSession checks that calls without a session fail before
// reaching the SDK.
func TestConfigOptionNoSession(t *testing.T) {
	lb := newLoopback(t)
	if _, err := lb.client.SetConfigOption(context.Background(), "", "model", "opus"); err == nil {
		t.Error("SetConfigOption with no session returned no error")
	}
	if _, err := lb.client.SetConfigOptionBool(context.Background(), "", "web_search", true); err == nil {
		t.Error("SetConfigOptionBool with no session returned no error")
	}
}

func sameChoices(got, want []ConfigChoice) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
