package migrate

import (
	"context"
	"errors"
	"testing"

	twasst "github.com/twilio/twilio-go/rest/assistants/v1"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

// --- fakes ---

type fakeAsstSource struct {
	assistants      []twasst.AssistantsV1Assistant
	tools           []twasst.AssistantsV1Tool
	knowledge       []twasst.AssistantsV1Knowledge
	toolsByAsst     map[string][]twasst.AssistantsV1Tool
	knowledgeByAsst map[string][]twasst.AssistantsV1Knowledge

	assistantErr, toolErr, knowledgeErr, toolByAsstErr, knowledgeByAsstErr error
}

func (f fakeAsstSource) ListAssistants(*twasst.ListAssistantsParams) ([]twasst.AssistantsV1Assistant, error) {
	if f.assistantErr != nil {
		return nil, f.assistantErr
	}
	return f.assistants, nil
}
func (f fakeAsstSource) ListTools(*twasst.ListToolsParams) ([]twasst.AssistantsV1Tool, error) {
	if f.toolErr != nil {
		return nil, f.toolErr
	}
	return f.tools, nil
}
func (f fakeAsstSource) ListKnowledge(*twasst.ListKnowledgeParams) ([]twasst.AssistantsV1Knowledge, error) {
	if f.knowledgeErr != nil {
		return nil, f.knowledgeErr
	}
	return f.knowledge, nil
}
func (f fakeAsstSource) ListToolsByAssistant(id string, _ *twasst.ListToolsByAssistantParams) ([]twasst.AssistantsV1Tool, error) {
	if f.toolByAsstErr != nil {
		return nil, f.toolByAsstErr
	}
	return f.toolsByAsst[id], nil
}
func (f fakeAsstSource) ListKnowledgeByAssistant(id string, _ *twasst.ListKnowledgeByAssistantParams) ([]twasst.AssistantsV1Knowledge, error) {
	if f.knowledgeByAsstErr != nil {
		return nil, f.knowledgeByAsstErr
	}
	return f.knowledgeByAsst[id], nil
}

type fakeAsstDest struct {
	existingAssistants []voiceml.AssistantsV1Assistant
	existingTools      []voiceml.AssistantsV1Tool
	existingKnowledge  []voiceml.AssistantsV1Knowledge
	existingAsstTools  map[string][]voiceml.AssistantsV1Tool
	existingAsstKnow   map[string][]voiceml.AssistantsV1Knowledge

	createdAssistants []voiceml.CreateAssistantRequest
	createdTools      []voiceml.CreateToolRequest
	createdKnowledge  []voiceml.CreateKnowledgeRequest
	attachedTools     map[string][]string
	attachedKnowledge map[string][]string

	listAssistantErr, createAssistantErr,
	listToolErr, createToolErr,
	listKnowledgeErr, createKnowledgeErr,
	listAsstToolErr, attachToolErr,
	listAsstKnowledgeErr, attachKnowledgeErr error

	nextIDByPrefix map[string]int
}

// newID returns a fresh, prefix-scoped ID (AS1, AS2, ...; TL1, TL2, ...;
// independent per resource type so expected IDs in test assertions are
// predictable regardless of call order across resource types).
func (f *fakeAsstDest) newID(prefix string) *string {
	if f.nextIDByPrefix == nil {
		f.nextIDByPrefix = map[string]int{}
	}
	f.nextIDByPrefix[prefix]++
	id := prefix + string(rune('0'+f.nextIDByPrefix[prefix]))
	return &id
}

func (f *fakeAsstDest) ListAssistants(context.Context, voiceml.ListAssistantsParams) (*voiceml.AssistantsV1AssistantList, error) {
	if f.listAssistantErr != nil {
		return nil, f.listAssistantErr
	}
	return &voiceml.AssistantsV1AssistantList{Assistants: f.existingAssistants}, nil
}
func (f *fakeAsstDest) CreateAssistant(_ context.Context, p *voiceml.CreateAssistantRequest) (*voiceml.AssistantsV1Assistant, error) {
	if f.createAssistantErr != nil {
		return nil, f.createAssistantErr
	}
	f.createdAssistants = append(f.createdAssistants, *p)
	return &voiceml.AssistantsV1Assistant{ID: f.newID("AS"), Name: &p.Name}, nil
}
func (f *fakeAsstDest) ListTools(context.Context, voiceml.ListToolsParams) (*voiceml.AssistantsV1ToolList, error) {
	if f.listToolErr != nil {
		return nil, f.listToolErr
	}
	return &voiceml.AssistantsV1ToolList{Tools: f.existingTools}, nil
}
func (f *fakeAsstDest) CreateTool(_ context.Context, p *voiceml.CreateToolRequest) (*voiceml.AssistantsV1Tool, error) {
	if f.createToolErr != nil {
		return nil, f.createToolErr
	}
	f.createdTools = append(f.createdTools, *p)
	return &voiceml.AssistantsV1Tool{ID: f.newID("TL"), Name: &p.Name}, nil
}
func (f *fakeAsstDest) ListKnowledge(context.Context, voiceml.ListKnowledgeParams) (*voiceml.AssistantsV1KnowledgeList, error) {
	if f.listKnowledgeErr != nil {
		return nil, f.listKnowledgeErr
	}
	return &voiceml.AssistantsV1KnowledgeList{Knowledge: f.existingKnowledge}, nil
}
func (f *fakeAsstDest) CreateKnowledge(_ context.Context, p *voiceml.CreateKnowledgeRequest) (*voiceml.AssistantsV1Knowledge, error) {
	if f.createKnowledgeErr != nil {
		return nil, f.createKnowledgeErr
	}
	f.createdKnowledge = append(f.createdKnowledge, *p)
	return &voiceml.AssistantsV1Knowledge{ID: f.newID("KN"), Name: &p.Name}, nil
}
func (f *fakeAsstDest) ListAssistantTools(_ context.Context, assistantID string, _ voiceml.V1PageParams) (*voiceml.AssistantsV1ToolList, error) {
	if f.listAsstToolErr != nil {
		return nil, f.listAsstToolErr
	}
	return &voiceml.AssistantsV1ToolList{Tools: f.existingAsstTools[assistantID]}, nil
}
func (f *fakeAsstDest) AttachToolToAssistant(_ context.Context, assistantID, toolID string) error {
	if f.attachToolErr != nil {
		return f.attachToolErr
	}
	if f.attachedTools == nil {
		f.attachedTools = map[string][]string{}
	}
	f.attachedTools[assistantID] = append(f.attachedTools[assistantID], toolID)
	return nil
}
func (f *fakeAsstDest) ListAssistantKnowledge(_ context.Context, assistantID string, _ voiceml.V1PageParams) (*voiceml.AssistantsV1KnowledgeList, error) {
	if f.listAsstKnowledgeErr != nil {
		return nil, f.listAsstKnowledgeErr
	}
	return &voiceml.AssistantsV1KnowledgeList{Knowledge: f.existingAsstKnow[assistantID]}, nil
}
func (f *fakeAsstDest) AttachKnowledgeToAssistant(_ context.Context, assistantID, knowledgeID string) error {
	if f.attachKnowledgeErr != nil {
		return f.attachKnowledgeErr
	}
	if f.attachedKnowledge == nil {
		f.attachedKnowledge = map[string][]string{}
	}
	f.attachedKnowledge[assistantID] = append(f.attachedKnowledge[assistantID], knowledgeID)
	return nil
}

// --- full success path ---

func fullAsstSource() fakeAsstSource {
	return fakeAsstSource{
		assistants: []twasst.AssistantsV1Assistant{{
			Id: "AAold", Name: "Support Bot", Owner: "Acme", Model: "gpt-4o", PersonalityPrompt: "Be helpful.",
			CustomerAi: map[string]interface{}{"perception_engine_enabled": true, "personalization_engine_enabled": false},
		}},
		tools:     []twasst.AssistantsV1Tool{{Id: "TLold", Name: "lookup", Type: "WEBHOOK", Enabled: true, Description: "looks stuff up", Meta: map[string]interface{}{"method": "GET"}}},
		knowledge: []twasst.AssistantsV1Knowledge{{Id: "KNold", Name: "faq", Type: "Text", Description: "FAQ doc", EmbeddingModel: "text-embedding-3", KnowledgeSourceDetails: map[string]interface{}{"text": "hello"}}},
		toolsByAsst: map[string][]twasst.AssistantsV1Tool{
			"AAold": {{Id: "TLold", Name: "lookup"}},
		},
		knowledgeByAsst: map[string][]twasst.AssistantsV1Knowledge{
			"AAold": {{Id: "KNold", Name: "faq"}},
		},
	}
}

func TestMigrateAssistants_FullSuccess(t *testing.T) {
	dst := &fakeAsstDest{}
	res, err := migrateAssistants(context.Background(), fullAsstSource(), dst, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(dst.createdAssistants) != 1 || dst.createdAssistants[0].Name != "Support Bot" {
		t.Errorf("assistant not created: %+v", dst.createdAssistants)
	}
	if dst.createdAssistants[0].CustomerAI == nil || dst.createdAssistants[0].CustomerAI.PerceptionEngineEnabled == nil || !*dst.createdAssistants[0].CustomerAI.PerceptionEngineEnabled {
		t.Errorf("customer AI not mapped: %+v", dst.createdAssistants[0].CustomerAI)
	}
	if len(dst.createdTools) != 1 || dst.createdTools[0].Name != "lookup" || dst.createdTools[0].Meta == nil {
		t.Errorf("tool not created: %+v", dst.createdTools)
	}
	if len(dst.createdKnowledge) != 1 || dst.createdKnowledge[0].Name != "faq" || dst.createdKnowledge[0].KnowledgeSourceDetails == nil {
		t.Errorf("knowledge not created: %+v", dst.createdKnowledge)
	}
	if len(dst.attachedTools["AS1"]) != 1 || dst.attachedTools["AS1"][0] != "TL1" {
		t.Errorf("tool not attached: %+v", dst.attachedTools)
	}
	if len(dst.attachedKnowledge["AS1"]) != 1 || dst.attachedKnowledge["AS1"][0] != "KN1" {
		t.Errorf("knowledge not attached: %+v", dst.attachedKnowledge)
	}
	if res.HasFailures() {
		t.Errorf("expected no failures, got %+v", res.Items)
	}
}

func TestMigrateAssistants_DryRun(t *testing.T) {
	dst := &fakeAsstDest{}
	res, err := migrateAssistants(context.Background(), fullAsstSource(), dst, Options{DryRun: true})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(dst.createdAssistants)+len(dst.createdTools)+len(dst.createdKnowledge) != 0 {
		t.Errorf("dry run must not create: %+v", dst)
	}
	if len(dst.attachedTools) != 0 || len(dst.attachedKnowledge) != 0 {
		t.Errorf("dry run must not attach: %+v / %+v", dst.attachedTools, dst.attachedKnowledge)
	}
	if res.Count(StatusPlanned) == 0 {
		t.Errorf("expected planned items, got %+v", res.Items)
	}
}

// --- empty-name failures ---

func TestMigrateAssistants_EmptyNames(t *testing.T) {
	src := fakeAsstSource{
		assistants: []twasst.AssistantsV1Assistant{{Id: "AA1"}},
		tools:      []twasst.AssistantsV1Tool{{Id: "TL1"}},
		knowledge:  []twasst.AssistantsV1Knowledge{{Id: "KN1"}},
	}
	res, err := migrateAssistants(context.Background(), src, &fakeAsstDest{}, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusFailed) != 3 {
		t.Errorf("expected 3 failed items, got %d: %+v", res.Count(StatusFailed), res.Items)
	}
}

// --- skips ---

func TestMigrateAssistants_SkipsExisting(t *testing.T) {
	src := fullAsstSource()
	dst := &fakeAsstDest{
		existingAssistants: []voiceml.AssistantsV1Assistant{{ID: strp("ASexist"), Name: strp("Support Bot")}},
		existingTools:      []voiceml.AssistantsV1Tool{{ID: strp("TLexist"), Name: strp("lookup")}},
		existingKnowledge:  []voiceml.AssistantsV1Knowledge{{ID: strp("KNexist"), Name: strp("faq")}},
	}
	dst.existingAsstTools = map[string][]voiceml.AssistantsV1Tool{"ASexist": {{ID: strp("TLexist")}}}
	dst.existingAsstKnow = map[string][]voiceml.AssistantsV1Knowledge{"ASexist": {{ID: strp("KNexist")}}}

	res, err := migrateAssistants(context.Background(), src, dst, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(dst.createdAssistants)+len(dst.createdTools)+len(dst.createdKnowledge) != 0 {
		t.Errorf("expected no creates, got %+v", dst)
	}
	if len(dst.attachedTools) != 0 || len(dst.attachedKnowledge) != 0 {
		t.Errorf("expected no new attachments (already attached), got %+v / %+v", dst.attachedTools, dst.attachedKnowledge)
	}
	if res.Count(StatusSkipped) == 0 {
		t.Errorf("expected skipped items, got %+v", res.Items)
	}
}

// --- create errors ---

func TestMigrateAssistants_CreateErrors(t *testing.T) {
	src := fullAsstSource()
	dst := &fakeAsstDest{
		createAssistantErr: errors.New("x"),
		createToolErr:      errors.New("x"),
		createKnowledgeErr: errors.New("x"),
	}
	res, err := migrateAssistants(context.Background(), src, dst, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusFailed) < 3 {
		t.Errorf("expected at least 3 failed items, got %d: %+v", res.Count(StatusFailed), res.Items)
	}
}

func TestMigrateAssistants_AttachErrors(t *testing.T) {
	src := fullAsstSource()
	dst := &fakeAsstDest{
		attachToolErr:      errors.New("x"),
		attachKnowledgeErr: errors.New("x"),
	}
	res, err := migrateAssistants(context.Background(), src, dst, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusFailed) < 2 {
		t.Errorf("expected at least 2 attach failures, got %d: %+v", res.Count(StatusFailed), res.Items)
	}
}

func TestMigrateAssistants_UnresolvableAttachments(t *testing.T) {
	// An assistant references tool/knowledge IDs that never showed up in the
	// top-level Tools/Knowledge lists at all.
	src := fakeAsstSource{
		assistants:      []twasst.AssistantsV1Assistant{{Id: "AAold", Name: "Bot"}},
		toolsByAsst:     map[string][]twasst.AssistantsV1Tool{"AAold": {{Id: "TLmissing", Name: "ghost-tool"}}},
		knowledgeByAsst: map[string][]twasst.AssistantsV1Knowledge{"AAold": {{Id: "KNmissing", Name: "ghost-knowledge"}}},
	}
	res, err := migrateAssistants(context.Background(), src, &fakeAsstDest{}, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusFailed) != 2 {
		t.Errorf("expected 2 unresolved-attachment failures, got %d: %+v", res.Count(StatusFailed), res.Items)
	}
}

// --- list error sweep ---

func TestMigrateAssistants_ListErrors(t *testing.T) {
	base := func() fakeAsstSource { return fullAsstSource() }
	cases := []struct {
		name string
		src  fakeAsstSource
		dst  *fakeAsstDest
	}{
		{"twilio assistants", func() fakeAsstSource { s := base(); s.assistantErr = errors.New("x"); return s }(), &fakeAsstDest{}},
		{"voiceml assistants", base(), &fakeAsstDest{listAssistantErr: errors.New("x")}},
		{"twilio tools", func() fakeAsstSource { s := base(); s.toolErr = errors.New("x"); return s }(), &fakeAsstDest{}},
		{"voiceml tools", base(), &fakeAsstDest{listToolErr: errors.New("x")}},
		{"twilio knowledge", func() fakeAsstSource { s := base(); s.knowledgeErr = errors.New("x"); return s }(), &fakeAsstDest{}},
		{"voiceml knowledge", base(), &fakeAsstDest{listKnowledgeErr: errors.New("x")}},
		{"twilio tools by assistant", func() fakeAsstSource { s := base(); s.toolByAsstErr = errors.New("x"); return s }(), &fakeAsstDest{}},
		{"voiceml assistant tools", base(), &fakeAsstDest{listAsstToolErr: errors.New("x")}},
		{"twilio knowledge by assistant", func() fakeAsstSource { s := base(); s.knowledgeByAsstErr = errors.New("x"); return s }(), &fakeAsstDest{}},
		{"voiceml assistant knowledge", base(), &fakeAsstDest{listAsstKnowledgeErr: errors.New("x")}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := migrateAssistants(context.Background(), c.src, c.dst, Options{}); err == nil {
				t.Errorf("%s: want error", c.name)
			}
		})
	}
}

// --- helper functions ---

func TestCustomerAIFields(t *testing.T) {
	if got := customerAIFields(nil); got != nil {
		t.Errorf("nil map should return nil, got %+v", got)
	}
	got := customerAIFields(map[string]interface{}{"perception_engine_enabled": true})
	if got == nil || got.PerceptionEngineEnabled == nil || !*got.PerceptionEngineEnabled {
		t.Errorf("perception flag not extracted: %+v", got)
	}
	if got.PersonalizationEngineEnabled == nil || *got.PersonalizationEngineEnabled {
		t.Errorf("missing personalization flag should default false: %+v", got)
	}
}

func TestAssistantsName(t *testing.T) {
	if (Assistants{}).Name() != "assistants" {
		t.Errorf("name=%q", (Assistants{}).Name())
	}
}
