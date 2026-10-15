package ai

import (
	"context"
	"testing"
	"time"

	"github.com/useopendesk/opendesk/internal/config"
	"github.com/useopendesk/opendesk/internal/deps"
	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
	"github.com/useopendesk/opendesk/internal/plan"
)

// pricedService is an OpenRouter service with a cached catalog: Sonnet at $2 / $10 and
// Haiku at $0.10 / $0.50 per million tokens, plus a model without a price.
func pricedService(billing bool) *Service {
	s := &Service{d: &deps.Deps{Cfg: &config.Config{}, Plan: plan.New(billing, 1.5)},
		provider: NewOpenRouter("http://openrouter.test/api", "k", "http://app.test")}
	s.models.Default, s.models.Fast = "anthropic/claude-sonnet-5.5", "anthropic/claude-haiku-5.5"
	s.catalog.info = map[string]ModelInfo{
		"anthropic/claude-sonnet-5.5": {ID: "anthropic/claude-sonnet-5.5", Name: "Claude Sonnet 5.5", inUSD: 2e-6, outUSD: 10e-6, priced: true},
		"anthropic/claude-haiku-5.5":  {ID: "anthropic/claude-haiku-5.5", Name: "Claude Haiku 5.5", inUSD: 0.1e-6, outUSD: 0.5e-6, priced: true},
		"anthropic/claude-opus-5.5":   {ID: "anthropic/claude-opus-5.5", Name: "Claude Opus 5.5", inUSD: 4e-6, outUSD: 20e-6, priced: true},
		"vendor/unpriced":             {ID: "vendor/unpriced", Name: "Unpriced"},
	}
	s.catalog.fetched = time.Now()
	return s
}

func TestCatalogPricesAsTheWorkspacePays(t *testing.T) {
	ctx := context.Background()
	s := pricedService(true)
	byID := map[string]ModelInfo{}
	for _, m := range s.Catalog(ctx) {
		byID[m.ID] = m
	}
	sonnet := byID["anthropic/claude-sonnet-5.5"]
	if sonnet.USDPerMTok == nil || sonnet.USDPerMTok.Input != 3 || sonnet.USDPerMTok.Output != 15 {
		t.Fatalf("sonnet price %+v", sonnet.USDPerMTok)
	}
	// A typical draft reply: 4,000 input and 500 output tokens at $3 / $15.
	if sonnet.TypicalReplyUSD == nil || *sonnet.TypicalReplyUSD != 0.0195 {
		t.Fatalf("sonnet typical reply %v", sonnet.TypicalReplyUSD)
	}
	haiku := byID["anthropic/claude-haiku-5.5"]
	if haiku.USDPerMTok.Input != 0.15 || haiku.USDPerMTok.Output != 0.75 || *haiku.TypicalReplyUSD != 0.000975 {
		t.Fatalf("haiku %+v %v", haiku.USDPerMTok, *haiku.TypicalReplyUSD)
	}
	// Without billing, the provider's price as it is.
	self := pricedService(false)
	for _, m := range self.Catalog(ctx) {
		if m.ID == "anthropic/claude-sonnet-5.5" && (m.USDPerMTok.Input != 2 || m.USDPerMTok.Output != 10) {
			t.Fatalf("self-hosted price %+v", m.USDPerMTok)
		}
	}
}

func TestEstimateFollowsTheModel(t *testing.T) {
	ctx := context.Background()
	s := pricedService(true)
	// 2,000 prompt tokens and 1,000 output tokens on Sonnet at 1.5x: $0.006 + $0.015.
	if got := s.estimate(ctx, "draft_reply", "anthropic/claude-sonnet-5.5", 2000, 1000); got != 21000 {
		t.Fatalf("sonnet estimate %d", got)
	}
	// A pricier model needs more balance to start the same call.
	if opus := s.estimate(ctx, "draft_reply", "anthropic/claude-opus-5.5", 2000, 1000); opus != 42000 {
		t.Fatalf("opus estimate %d", opus)
	}
	// Output is counted up to estimateMaxOutput.
	if got := s.estimate(ctx, "translate", "anthropic/claude-haiku-5.5", 0, 12000); got != plan.FromCost(estimateMaxOutput*0.5e-6, 1.5) {
		t.Fatalf("capped estimate %d", got)
	}
	// An unknown price falls back to the feature's typical cost.
	if got := s.estimate(ctx, "kb_draft", "vendor/unpriced", 8000, 10000); got != plan.Typical["kb_draft"] {
		t.Fatalf("unpriced estimate %d", got)
	}
}

func TestFeatureCostsUseTheChosenModels(t *testing.T) {
	ctx := context.Background()
	s := pricedService(true)
	s.d.Cfg.AIModels = []string{"anthropic/claude-opus-5.5"}
	costs := map[string]float64{}
	for k, v := range plan.Typical {
		costs[k] = v.USD()
	}
	s.featureCosts(ctx, hd.AISettings{}, costs, false)
	if costs["draft_reply"] != 0.0195 || costs["answer_check"] != 0.001 {
		t.Fatalf("default costs %v", costs)
	}
	// Rewrite runs on the fast model: 800 input and 400 output tokens on Haiku at 1.5x.
	if costs["rewrite"] != 0.00042 {
		t.Fatalf("rewrite %v", costs["rewrite"])
	}
	s.featureCosts(ctx, hd.AISettings{Model: "anthropic/claude-opus-5.5"}, costs, false)
	if costs["draft_reply"] != 0.039 || costs["rewrite"] != 0.00042 {
		t.Fatalf("opus costs %v", costs)
	}
	// On the workspace's own key, LLM features cost the AI balance nothing; Jev's do.
	s.featureCosts(ctx, hd.AISettings{}, costs, true)
	if costs["draft_reply"] != 0 || costs["answer_check"] != 0.001 {
		t.Fatalf("own key costs %v", costs)
	}
}

func TestPromptTokens(t *testing.T) {
	req := Request{System: []Block{{Text: "12345678"}}, Messages: []Message{UserText("abcd", "ef")}}
	if n := requestTokens(req); n != 4 {
		t.Fatalf("request tokens %d", n)
	}
	chat := ChatRequest{Turns: []ChatTurn{{Role: "user", Text: "abcdefgh"}, {Role: "user", Results: []ToolResult{{Content: "abcd"}}}}}
	if n := chatTokens(chat); n != 3 {
		t.Fatalf("chat tokens %d", n)
	}
}
