package ai

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/jobs"
	"github.com/useopendesk/opendesk/internal/plan"
)

// fakeSnippets stands in for internal/ai/knowledge: it retrieves nothing and records
// the snippets "Improve answer" creates.
type fakeSnippets struct {
	mu       sync.Mutex
	snippets []fakeSnippet
}

type fakeSnippet struct {
	Name, Question, Answer string
	By                     uuid.UUID
}

func (f *fakeSnippets) Retrieve(context.Context, uuid.UUID, string, int) ([]Passage, error) {
	return nil, nil
}

func (f *fakeSnippets) CreateSnippet(_ context.Context, p *auth.Principal, name, question, answer string) (uuid.UUID, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snippets = append(f.snippets, fakeSnippet{Name: name, Question: question, Answer: answer, By: p.UserID})
	return uuid.New(), question, nil
}

func (e *env) runTestJobs() {
	e.T.Helper()
	for _, raw := range e.Jobs(jobs.AIAgentTestRunArgs{}.Kind(), e.WS.ID) {
		var a jobs.AIAgentTestRunArgs
		_ = json.Unmarshal(raw, &a)
		w := &AgentTestRunWorker{S: e.S}
		if err := w.Work(e.Ctx, &river.Job[jobs.AIAgentTestRunArgs]{JobRow: &rivertype.JobRow{}, Args: a}); err != nil {
			e.T.Fatalf("test run: %v", err)
		}
	}
}

func (e *env) createTest(body map[string]any) AgentTest {
	e.T.Helper()
	var t AgentTest
	e.Do("POST", "/v1/ai/agent/tests", e.admin(), body).Want(e.T, 201).JSON(e.T, &t)
	return t
}

func (e *env) startRun(body map[string]any) AgentTestRun {
	e.T.Helper()
	var r AgentTestRun
	e.Do("POST", "/v1/ai/agent/test-runs", e.admin(), body).Want(e.T, 202).JSON(e.T, &r)
	return r
}

func (e *env) getRun(id uuid.UUID) AgentTestRun {
	e.T.Helper()
	var r AgentTestRun
	e.Do("GET", "/v1/ai/agent/test-runs/"+id.String(), e.admin(), nil).Want(e.T, 200).JSON(e.T, &r)
	return r
}

func resultOf(t *testing.T, run AgentTestRun, id uuid.UUID) AgentTestResult {
	t.Helper()
	for _, r := range run.Results {
		if r.TestID == id {
			return r
		}
	}
	t.Fatalf("no result for %s in %+v", id, run.Results)
	return AgentTestResult{}
}

func visitorMsg(text string) []any { return []any{map[string]any{"role": "visitor", "text": text}} }

func TestAgentTests(t *testing.T) {
	e := newEnv(t, opts{billing: true})
	// One test at a time, so the scripted replies arrive in order.
	agentRunConcurrency = 1
	t.Cleanup(func() { agentRunConcurrency = 3 })
	art := e.article("Reset your password", "<p>To reset your password, open the sign-in page and choose Forgot password.</p>", "published")
	if err := e.S.Index(e.Ctx, e.WS.ID, "article", art); err != nil {
		t.Fatal(err)
	}

	// Admins only; validation.
	body := map[string]any{"messages": visitorMsg("How do I reset my password?"), "expectations": []any{
		map[string]any{"kind": "mentions", "value": "the Forgot password link on the sign-in page"},
		map[string]any{"kind": "not_mentions", "value": "contacting support by phone"},
		map[string]any{"kind": "no_handoff"},
		map[string]any{"kind": "cites_knowledge"},
	}}
	e.Do("POST", "/v1/ai/agent/tests", e.agent(), body).Want(t, 403)
	e.Do("GET", "/v1/ai/agent/tests", e.viewer(), nil).Want(t, 403)
	for _, bad := range []map[string]any{
		{"messages": []any{map[string]any{"role": "agent", "text": "Hi"}}, "expectations": []any{map[string]any{"kind": "handoff"}}},
		{"messages": visitorMsg("Hi"), "expectations": []any{}},
		{"messages": visitorMsg("Hi"), "expectations": []any{map[string]any{"kind": "mentions"}}},
		{"messages": visitorMsg("Hi"), "expectations": []any{map[string]any{"kind": "handoff"}, map[string]any{"kind": "no_handoff"}}},
		{"messages": visitorMsg("Hi"), "expectations": []any{map[string]any{"kind": "uses_tool", "value": "not a tool!"}}},
		{"messages": []any{map[string]any{"role": "visitor", "text": "1"}, map[string]any{"role": "agent", "text": "2"}, map[string]any{"role": "visitor", "text": "3"},
			map[string]any{"role": "agent", "text": "4"}, map[string]any{"role": "visitor", "text": "5"}, map[string]any{"role": "agent", "text": "6"},
			map[string]any{"role": "visitor", "text": "7"}}, "expectations": []any{map[string]any{"kind": "handoff"}}},
	} {
		e.Do("POST", "/v1/ai/agent/tests", e.admin(), bad).Want(t, 422)
	}
	_, seq := e.Events(e.WS.ID, 0)
	pass := e.createTest(body)
	if pass.Name != "How do I reset my password?" || len(pass.Expectations) != 4 || pass.LastResult != nil || pass.Visitor != nil {
		t.Fatalf("test %+v", pass)
	}
	if types, _ := e.Events(e.WS.ID, seq); len(types) != 1 || types[0] != "ai_agent_test.created" {
		t.Fatalf("events %v", types)
	}
	if n := e.count(`SELECT count(*) FROM audit_logs WHERE workspace_id = $1 AND action = 'ai.agent_test_created'`, e.WS.ID); n != 1 {
		t.Fatalf("audit %d", n)
	}
	handoff := e.createTest(map[string]any{"name": "  Refund   exception ", "messages": visitorMsg("I want a refund for last year"),
		"expectations": []any{map[string]any{"kind": "handoff"}, map[string]any{"kind": "mentions", "value": "a person will follow up"}}})
	if handoff.Name != "Refund exception" {
		t.Fatalf("name %q", handoff.Name)
	}

	// Run all against the saved settings: one passes, one fails.
	e.F.queue("widget", "To reset your password, choose Forgot password on the sign-in page [1].", "I can help with that. Refunds are processed in 5 days.")
	e.F.queue("judge",
		`{"results":[{"n":1,"met":true,"reason":"It explains the Forgot password link."},{"n":2,"met":true,"reason":"No phone support."}]}`,
		`{"results":[{"n":1,"met":false,"reason":"It does not say a person will follow up."}]}`)
	run := e.startRun(map[string]any{})
	if run.Status != "queued" || run.Total != 2 || len(run.TestIDs) != 2 || run.Draft {
		t.Fatalf("run %+v", run)
	}
	e.Do("POST", "/v1/ai/agent/test-runs", e.admin(), map[string]any{}).Want(t, 409)
	_, seq = e.Events(e.WS.ID, 0)
	e.runTestJobs()
	got := e.getRun(run.ID)
	if got.Status != "done" || got.Passed != 1 || got.Failed != 1 || got.Errored != 0 || got.USD <= 0 || got.FinishedAt == nil {
		t.Fatalf("run %+v", got)
	}
	r1 := resultOf(t, got, pass.ID)
	if r1.Status != "passed" || len(r1.Checks) != 4 || !strings.Contains(r1.AnswerHTML, "sign-in page") || len(r1.Sources) != 1 || r1.Handoff {
		t.Fatalf("passing result %+v", r1)
	}
	for _, c := range r1.Checks {
		if !c.Passed || c.Reason == "" {
			t.Fatalf("check %+v", c)
		}
	}
	r2 := resultOf(t, got, handoff.ID)
	if r2.Status != "failed" || r2.Checks[0].Passed || r2.Checks[1].Passed || r2.Checks[1].Reason != "It does not say a person will follow up." {
		t.Fatalf("failing result %+v", r2)
	}
	// The judge ran on the fast model, with the answer and expectations as data.
	var judgeReq map[string]any
	e.F.mu.Lock()
	for _, rq := range e.F.requests {
		if sys, _ := json.Marshal(rq["system"]); strings.Contains(string(sys), "You grade answers") {
			judgeReq = rq
		}
	}
	e.F.mu.Unlock()
	if judgeReq == nil || judgeReq["model"] != "claude-haiku-4-5-20251001" || !strings.Contains(userText(judgeReq), `<expectation n="1" type="conveys">`) {
		t.Fatalf("judge request %v", judgeReq)
	}
	types, _ := e.Events(e.WS.ID, seq)
	if !containsStr(types, "ai_agent_test_result.updated") || types[len(types)-1] != "ai_agent_test_run.updated" {
		t.Fatalf("events %v", types)
	}
	// The AI usage is playground usage; nothing reaches the report or the tool log.
	if n := e.count(`SELECT count(*) FROM ai_usage WHERE workspace_id = $1 AND feature <> 'playground'`, e.WS.ID); n != 0 {
		t.Fatalf("non-playground usage %d", n)
	}
	var list agentTestList
	e.Do("GET", "/v1/ai/agent/tests", e.admin(), nil).Want(t, 200).JSON(t, &list)
	if len(list.Data) != 2 || list.Data[0].LastResult == nil || list.Data[0].LastResult.Status != "passed" || list.Data[0].LastResult.PreviousStatus != nil {
		t.Fatalf("list %+v", list.Data)
	}

	// Run one test against a draft: the draft instructions reach the agent, and the
	// test that passed before now fails (it stands out with previous_status).
	e.F.queue("widget", "Please call our support line to reset it.")
	e.F.queue("judge", `{"results":[{"n":1,"met":false,"reason":"No Forgot password link."},{"n":2,"met":false,"reason":"It says to call support."}]}`)
	draft := e.startRun(map[string]any{"test_ids": []any{pass.ID}, "settings": map[string]any{"bot": map[string]any{"personality": map[string]any{"instructions": "Send people to the phone line."}}}})
	if !draft.Draft || draft.Total != 1 {
		t.Fatalf("draft run %+v", draft)
	}
	e.runTestJobs()
	if sys, _ := json.Marshal(e.F.requests[len(e.F.requests)-2]["system"]); !strings.Contains(string(sys), "Send people to the phone line.") {
		t.Fatal("draft instructions not used")
	}
	got = e.getRun(draft.ID)
	r3 := resultOf(t, got, pass.ID)
	if got.Status != "done" || r3.Status != "failed" || !r3.Draft || r3.PreviousStatus == nil || *r3.PreviousStatus != "passed" || r3.Checks[2].Passed != true || r3.Checks[3].Passed {
		t.Fatalf("draft result %+v %+v", got, r3)
	}

	// Tools: a read-only tool runs; uses_tool passes; a confirm tool never runs.
	tb := &fakeToolbox{server: uuid.New()}
	e.S.SetToolbox(tb)
	t.Cleanup(func() { e.S.SetToolbox(nil) })
	tool := e.createTest(map[string]any{"messages": visitorMsg("Cancel order 42"), "expectations": []any{
		map[string]any{"kind": "uses_tool", "value": "shop__get_order"}, map[string]any{"kind": "uses_tool", "value": "shop__cancel_order"},
		map[string]any{"kind": "uses_tool", "value": "shop__refund"}}})
	e.F.queue("widget", "@tool shop__get_order {\"id\":\"42\"}", "Confirm below.\n@tool shop__cancel_order {\"id\":\"42\",\"_summary\":\"Cancel order 42\"}")
	tr := e.startRun(map[string]any{"test_ids": []any{tool.ID}})
	e.runTestJobs()
	r4 := resultOf(t, e.getRun(tr.ID), tool.ID)
	if r4.Status != "failed" || !r4.Checks[0].Passed || !r4.Checks[1].Passed || r4.Checks[2].Passed || r4.Action == nil ||
		r4.Checks[2].Reason != "This tool is not connected or not turned on." {
		t.Fatalf("tool result %+v", r4)
	}
	if calls := tb.called(); len(calls) != 1 || calls[0].Tool != "get_order" {
		t.Fatalf("tool calls %+v", calls)
	}

	// AI balance used up: the test errors and the run fails with the reason.
	e.Exec(`UPDATE ai_balances SET included_used_micro_usd = $2 WHERE workspace_id = $1`, e.WS.ID, plan.FreeMonthlyUSD)
	nc := e.startRun(map[string]any{"test_ids": []any{pass.ID}})
	e.runTestJobs()
	got = e.getRun(nc.ID)
	if got.Status != "failed" || got.Error == nil || got.Errored != 1 || got.Results[0].Error == nil || *got.Results[0].Error != "The AI balance is too low." {
		t.Fatalf("no balance run %+v", got)
	}

	// Cancelling a queued run: the job does nothing.
	cr := e.startRun(map[string]any{})
	var cancelled AgentTestRun
	e.Do("POST", "/v1/ai/agent/test-runs/"+cr.ID.String()+"/cancel", e.admin(), nil).Want(t, 200).JSON(t, &cancelled)
	before := e.F.calls()
	e.runTestJobs()
	if cancelled.Status != "cancelled" || e.F.calls() != before || e.getRun(cr.ID).Status != "cancelled" {
		t.Fatalf("cancelled %+v", cancelled)
	}

	// Update and delete.
	var upd AgentTest
	e.Do("PATCH", "/v1/ai/agent/tests/"+handoff.ID.String(), e.admin(), map[string]any{"name": "Refunds", "visitor": map[string]any{"verified": true, "user_id": "u_1"}}).
		Want(t, 200).JSON(t, &upd)
	if upd.Name != "Refunds" || upd.Visitor == nil || !upd.Visitor.Verified || upd.LastResult == nil {
		t.Fatalf("update %+v", upd)
	}
	e.Do("DELETE", "/v1/ai/agent/tests/"+handoff.ID.String(), e.admin(), nil).Want(t, 204)
	e.Do("GET", "/v1/ai/agent/tests/"+handoff.ID.String(), e.admin(), nil).Want(t, 404)

	// Another workspace sees none of it.
	other := e.NewWorkspace()
	e.Do("GET", "/v1/ai/agent/tests/"+pass.ID.String(), other.Admin.As(other), nil).Want(t, 404)
	e.Do("GET", "/v1/ai/agent/test-runs/"+run.ID.String(), other.Admin.As(other), nil).Want(t, 404)
}

func TestAgentTestLimit(t *testing.T) {
	e := newEnv(t, opts{})
	for i := 0; i < maxAgentTests; i++ {
		e.Exec(`INSERT INTO ai_agent_tests (workspace_id, name, messages, expectations) VALUES ($1, 't', '[{"role":"visitor","text":"Hi"}]', '[{"kind":"handoff"}]')`, e.WS.ID)
	}
	e.Do("POST", "/v1/ai/agent/tests", e.admin(), map[string]any{"messages": visitorMsg("Hi"), "expectations": []any{map[string]any{"kind": "handoff"}}}).Want(t, 422)
}

func TestImproveAnswerAndReview(t *testing.T) {
	e := newEnv(t, opts{billing: true})
	sn := &fakeSnippets{}
	e.S.SetKnowledge(sn)
	t.Cleanup(func() { e.S.SetKnowledge(nil) })
	e.settings(map[string]any{"widget_answers": true})

	// A reply the visitor rated down, and one where the agent handed off.
	e.F.queue("widget", "We do not ship to Australia.")
	va, tid, _ := widgetConversation(e, "Do you ship notebooks to Australia?")
	e.runWidgetJobs()
	reply := lastMessage(e, tid)
	e.Do("POST", "/v1/widget/conversations/"+tid.String()+"/messages/"+reply.ID.String()+"/rating", va, map[string]any{"rating": "down"}).Want(t, 204)
	e.F.queue("widget", "A person from the team will reply here.\n@tool handoff_to_human {\"reason\":\"invoice\"}")
	_, tid2, _ := widgetConversation(e, "Can I get an invoice with my VAT number?")
	e.runWidgetJobs()
	handed := lastMessage(e, tid2)
	// A visitor asking for a person is not a knowledge gap.
	_, _, _ = widgetConversation(e, "I want to talk to a real person please")
	e.runWidgetJobs()

	var q reviewList
	e.Do("GET", "/v1/ai/agent/review", e.agent(), nil).Want(t, 403)
	e.Do("GET", "/v1/ai/agent/review", e.admin(), nil).Want(t, 200).JSON(t, &q)
	if len(q.Data) != 2 || q.Data[0].ID != handed.ID || q.Data[0].Reason != "handoff" || q.Data[1].ID != reply.ID || q.Data[1].Reason != "rated_down" ||
		q.Data[1].Question != "Do you ship notebooks to Australia?" || !strings.Contains(q.Data[1].Answer, "Australia") {
		t.Fatalf("queue %+v", q.Data)
	}

	// Improve the rated reply: a snippet, a test, and it leaves the queue.
	e.Do("POST", "/v1/ai/agent/improve", e.agent(), map[string]any{"question": "Do you ship?", "answer": "Yes"}).Want(t, 403)
	e.Do("POST", "/v1/ai/agent/improve", e.admin(), map[string]any{"question": "Do", "answer": "Yes"}).Want(t, 422)
	e.Do("POST", "/v1/ai/agent/improve", e.admin(), map[string]any{"question": "Do you ship?", "answer": "Yes", "message_id": uuid.New()}).Want(t, 404)
	var out ImproveOutput
	e.Do("POST", "/v1/ai/agent/improve", e.admin(), map[string]any{"question": "Do you ship notebooks to Australia?",
		"answer": "Yes, we ship to **Australia** in 7 to 10 days.", "message_id": reply.ID, "save_test": true}).Want(t, 201).JSON(t, &out)
	if len(sn.snippets) != 1 || sn.snippets[0].Question != "Do you ship notebooks to Australia?" || sn.snippets[0].Answer != "Yes, we ship to **Australia** in 7 to 10 days." ||
		sn.snippets[0].By != e.WS.Admin.ID || out.Source.Kind != "snippet" {
		t.Fatalf("snippet %+v %+v", sn.snippets, out)
	}
	if out.Test == nil || out.Test.Messages[0].Text != "Do you ship notebooks to Australia?" || out.Test.Expectations[0].Kind != "mentions" ||
		out.Test.Expectations[0].Value != "Yes, we ship to Australia in 7 to 10 days." {
		t.Fatalf("test %+v", out.Test)
	}
	if n := e.count(`SELECT count(*) FROM audit_logs WHERE workspace_id = $1 AND action = 'ai.agent_answer_improved'`, e.WS.ID); n != 1 {
		t.Fatalf("audit %d", n)
	}
	// A long question becomes a snippet question of at most 300 characters.
	long := strings.Repeat("where is my parcel ", 30)
	e.Do("POST", "/v1/ai/agent/improve", e.admin(), map[string]any{"question": long, "answer": "Check the tracking link."}).Want(t, 201)
	if q := sn.snippets[1].Question; len([]rune(q)) > 300 || !strings.HasSuffix(q, "…") {
		t.Fatalf("snippet question %q", q)
	}

	// Dismiss the handoff.
	e.Do("POST", "/v1/ai/agent/review/"+uuid.New().String()+"/dismiss", e.admin(), nil).Want(t, 404)
	e.Do("POST", "/v1/ai/agent/review/"+handed.ID.String()+"/dismiss", e.admin(), nil).Want(t, 204)
	e.Do("GET", "/v1/ai/agent/review", e.admin(), nil).Want(t, 200).JSON(t, &q)
	if len(q.Data) != 0 {
		t.Fatalf("queue after %+v", q.Data)
	}
	var outcome string
	e.Scalar(`SELECT outcome FROM ai_agent_reviews WHERE message_id = $1`, &outcome, reply.ID)
	if outcome != "improved" {
		t.Fatalf("outcome %s", outcome)
	}
	// Another workspace can't touch these replies.
	other := e.NewWorkspace()
	e.Do("POST", "/v1/ai/agent/review/"+reply.ID.String()+"/dismiss", other.Admin.As(other), nil).Want(t, 404)
	_ = domain.Message{}
}
