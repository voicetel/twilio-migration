package migrate

import (
	"context"
	"encoding/json"
	"fmt"

	twasst "github.com/twilio/twilio-go/rest/assistants/v1"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

// assistantsSource is the slice of twilio-go used to read the Assistants
// product's configuration surface.
type assistantsSource interface {
	ListAssistants(params *twasst.ListAssistantsParams) ([]twasst.AssistantsV1Assistant, error)
	ListTools(params *twasst.ListToolsParams) ([]twasst.AssistantsV1Tool, error)
	ListKnowledge(params *twasst.ListKnowledgeParams) ([]twasst.AssistantsV1Knowledge, error)
	ListToolsByAssistant(assistantID string, params *twasst.ListToolsByAssistantParams) ([]twasst.AssistantsV1Tool, error)
	ListKnowledgeByAssistant(assistantID string, params *twasst.ListKnowledgeByAssistantParams) ([]twasst.AssistantsV1Knowledge, error)
}

// assistantsDest is the slice of voiceml-go-sdk used to read/write the
// Assistants product's configuration surface.
type assistantsDest interface {
	ListAssistants(ctx context.Context, params voiceml.ListAssistantsParams) (*voiceml.AssistantsV1AssistantList, error)
	CreateAssistant(ctx context.Context, params *voiceml.CreateAssistantRequest) (*voiceml.AssistantsV1Assistant, error)
	ListTools(ctx context.Context, params voiceml.ListToolsParams) (*voiceml.AssistantsV1ToolList, error)
	CreateTool(ctx context.Context, params *voiceml.CreateToolRequest) (*voiceml.AssistantsV1Tool, error)
	ListKnowledge(ctx context.Context, params voiceml.ListKnowledgeParams) (*voiceml.AssistantsV1KnowledgeList, error)
	CreateKnowledge(ctx context.Context, params *voiceml.CreateKnowledgeRequest) (*voiceml.AssistantsV1Knowledge, error)
	ListAssistantTools(ctx context.Context, assistantID string, params voiceml.V1PageParams) (*voiceml.AssistantsV1ToolList, error)
	AttachToolToAssistant(ctx context.Context, assistantID, toolID string) error
	ListAssistantKnowledge(ctx context.Context, assistantID string, params voiceml.V1PageParams) (*voiceml.AssistantsV1KnowledgeList, error)
	AttachKnowledgeToAssistant(ctx context.Context, assistantID, knowledgeID string) error
}

// Assistants migrates the Assistants product's CONFIGURATION surface:
// Assistants, standalone Tools and Knowledge sources (idempotent by name),
// and each Assistant's Tool/Knowledge attachments.
//
// Deliberately out of scope, verified against both SDKs — not scope
// preferences, but things that cannot be migrated at all:
//   - SegmentCredential (Segment.com analytics API key/write key): Twilio's
//     read side never returns it — write-only, nothing to copy. Same
//     category as SIP/Conversations credentials, except VoiceML also has no
//     way to mint a substitute the way SIP passwords can be regenerated.
//   - Policies (Tool/Knowledge access policies): voiceml-go-sdk's
//     AssistantsV1Service exposes only ListPolicies — no Create. Twilio
//     creates them inline via CreateToolRequest.Policy /
//     CreateKnowledgeRequest.Policy, but VoiceML's CreateToolRequest /
//     CreateKnowledgeRequest have no equivalent field. No write path exists.
//   - Sessions, Messages, Feedback: the only write endpoint on either side
//     (SendMessage / CreateMessage, "POST .../Messages") is a LIVE
//     LLM-execution call — it posts a prompt and returns a freshly generated
//     AI response, not a stored historical record. There is no
//     import-a-past-message endpoint. "Migrating" history through it would
//     mean re-running old prompts through the (possibly different) BYO-LLM
//     and getting back new, different responses — not copying data. This is
//     a technical impossibility, not a scope choice.
type Assistants struct{}

// Name implements Migrator.
func (Assistants) Name() string { return "assistants" }

// Migrate implements Migrator.
func (Assistants) Migrate(ctx context.Context, c *Clients, opts Options) (Result, error) {
	return migrateAssistants(ctx, c.TwilioAssistants, c.VoiceML.AssistantsV1, opts)
}

func migrateAssistants(ctx context.Context, src assistantsSource, dst assistantsDest, opts Options) (Result, error) {
	res := Result{Resource: "assistants"}

	assistants, err := src.ListAssistants(&twasst.ListAssistantsParams{})
	if err != nil {
		return res, fmt.Errorf("list Twilio assistants: %w", err)
	}
	assistantIDByOldID, err := migrateAsstAssistants(ctx, assistants, dst, opts, &res)
	if err != nil {
		return res, err
	}
	toolIDByName, err := migrateAsstTools(ctx, src, dst, opts, &res)
	if err != nil {
		return res, err
	}
	knowledgeIDByName, err := migrateAsstKnowledge(ctx, src, dst, opts, &res)
	if err != nil {
		return res, err
	}

	if opts.DryRun {
		// Attachments need a real new-side assistant ID to query/write
		// against; a brand-new assistant never gets one under DryRun (see
		// the identical reasoning in migrateConvConversations), so there is
		// nothing further to plan here beyond what's already recorded above.
		return res, nil
	}

	for _, a := range assistants {
		newAssistantID, ok := assistantIDByOldID[a.Id]
		if !ok {
			continue
		}
		if err := migrateAsstToolAttachments(ctx, src, dst, a.Id, a.Name, newAssistantID, toolIDByName, &res); err != nil {
			return res, err
		}
		if err := migrateAsstKnowledgeAttachments(ctx, src, dst, a.Id, a.Name, newAssistantID, knowledgeIDByName, &res); err != nil {
			return res, err
		}
	}

	return res, nil
}

// customerAIFields extracts the two known booleans from Twilio's untyped
// CustomerAi map into VoiceML's typed struct. Missing/malformed fields come
// back false (VoiceML's zero value), matching the SDK's own defaults.
func customerAIFields(m map[string]interface{}) *voiceml.AssistantsV1CustomerAI {
	if m == nil {
		return nil
	}
	perception, _ := m["perception_engine_enabled"].(bool)
	personalization, _ := m["personalization_engine_enabled"].(bool)
	return &voiceml.AssistantsV1CustomerAI{
		PerceptionEngineEnabled:      &perception,
		PersonalizationEngineEnabled: &personalization,
	}
}

// migrateAsstAssistants migrates Assistants, idempotent by name. Returns a
// Twilio-ID→VoiceML-ID map for the attachment phase.
func migrateAsstAssistants(ctx context.Context, assistants []twasst.AssistantsV1Assistant, dst assistantsDest, opts Options, res *Result) (map[string]string, error) {
	existing, err := dst.ListAssistants(ctx, voiceml.ListAssistantsParams{})
	if err != nil {
		return nil, fmt.Errorf("list VoiceML assistants: %w", err)
	}
	idByName := make(map[string]string, len(existing.Assistants))
	for _, a := range existing.Assistants {
		idByName[deref(a.Name)] = deref(a.ID)
	}

	idByOldID := make(map[string]string, len(assistants))
	for _, a := range assistants {
		id := "assistant " + a.Name

		switch {
		case a.Name == "":
			res.Items = append(res.Items, ItemResult{ID: "assistant", Status: StatusFailed, Detail: "source assistant has no name"})
			continue
		case idByName[a.Name] != "":
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusSkipped, Detail: "already present on VoiceML"})
		case opts.DryRun:
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusPlanned})
			continue
		default:
			var model *string
			if a.Model != "" {
				model = &a.Model
			}
			var owner *string
			if a.Owner != "" {
				owner = &a.Owner
			}
			var prompt *string
			if a.PersonalityPrompt != "" {
				prompt = &a.PersonalityPrompt
			}
			created, cErr := dst.CreateAssistant(ctx, &voiceml.CreateAssistantRequest{
				Name:              a.Name,
				Owner:             owner,
				PersonalityPrompt: prompt,
				Model:             model,
				CustomerAI:        customerAIFields(a.CustomerAi),
			})
			if cErr != nil {
				res.Items = append(res.Items, ItemResult{ID: id, Status: StatusFailed, Detail: cErr.Error()})
				continue
			}
			idByName[a.Name] = deref(created.ID)
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusCreated})
		}
		idByOldID[a.Id] = idByName[a.Name]
	}
	return idByOldID, nil
}

// migrateAsstTools migrates standalone Tools, idempotent by name. Returns a
// name→new-ID map for the attachment phase.
func migrateAsstTools(ctx context.Context, src assistantsSource, dst assistantsDest, opts Options, res *Result) (map[string]string, error) {
	tools, err := src.ListTools(&twasst.ListToolsParams{})
	if err != nil {
		return nil, fmt.Errorf("list Twilio tools: %w", err)
	}
	existing, err := dst.ListTools(ctx, voiceml.ListToolsParams{})
	if err != nil {
		return nil, fmt.Errorf("list VoiceML tools: %w", err)
	}
	idByName := make(map[string]string, len(existing.Tools))
	for _, t := range existing.Tools {
		idByName[deref(t.Name)] = deref(t.ID)
	}

	for _, t := range tools {
		id := "tool " + t.Name

		switch {
		case t.Name == "":
			res.Items = append(res.Items, ItemResult{ID: "tool", Status: StatusFailed, Detail: "source tool has no name"})
		case idByName[t.Name] != "":
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusSkipped, Detail: "already present on VoiceML"})
		case opts.DryRun:
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusPlanned})
		default:
			var desc *string
			if t.Description != "" {
				desc = &t.Description
			}
			var meta json.RawMessage
			if t.Meta != nil {
				if b, mErr := json.Marshal(t.Meta); mErr == nil {
					meta = b
				}
			}
			created, cErr := dst.CreateTool(ctx, &voiceml.CreateToolRequest{
				Name:        t.Name,
				Type:        t.Type,
				Enabled:     t.Enabled,
				Description: desc,
				Meta:        meta,
			})
			if cErr != nil {
				res.Items = append(res.Items, ItemResult{ID: id, Status: StatusFailed, Detail: cErr.Error()})
			} else {
				idByName[t.Name] = deref(created.ID)
				res.Items = append(res.Items, ItemResult{ID: id, Status: StatusCreated})
			}
		}
	}
	return idByName, nil
}

// migrateAsstKnowledge migrates standalone Knowledge sources, idempotent by
// name. Returns a name→new-ID map for the attachment phase.
func migrateAsstKnowledge(ctx context.Context, src assistantsSource, dst assistantsDest, opts Options, res *Result) (map[string]string, error) {
	sources, err := src.ListKnowledge(&twasst.ListKnowledgeParams{})
	if err != nil {
		return nil, fmt.Errorf("list Twilio knowledge: %w", err)
	}
	existing, err := dst.ListKnowledge(ctx, voiceml.ListKnowledgeParams{})
	if err != nil {
		return nil, fmt.Errorf("list VoiceML knowledge: %w", err)
	}
	idByName := make(map[string]string, len(existing.Knowledge))
	for _, k := range existing.Knowledge {
		idByName[deref(k.Name)] = deref(k.ID)
	}

	for _, k := range sources {
		id := "knowledge " + k.Name

		switch {
		case k.Name == "":
			res.Items = append(res.Items, ItemResult{ID: "knowledge", Status: StatusFailed, Detail: "source knowledge has no name"})
		case idByName[k.Name] != "":
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusSkipped, Detail: "already present on VoiceML"})
		case opts.DryRun:
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusPlanned})
		default:
			var desc *string
			if k.Description != "" {
				desc = &k.Description
			}
			var embedModel *string
			if k.EmbeddingModel != "" {
				embedModel = &k.EmbeddingModel
			}
			var details json.RawMessage
			if k.KnowledgeSourceDetails != nil {
				if b, mErr := json.Marshal(k.KnowledgeSourceDetails); mErr == nil {
					details = b
				}
			}
			created, cErr := dst.CreateKnowledge(ctx, &voiceml.CreateKnowledgeRequest{
				Name:                   k.Name,
				Type:                   k.Type,
				Description:            desc,
				EmbeddingModel:         embedModel,
				KnowledgeSourceDetails: details,
			})
			if cErr != nil {
				res.Items = append(res.Items, ItemResult{ID: id, Status: StatusFailed, Detail: cErr.Error()})
			} else {
				idByName[k.Name] = deref(created.ID)
				res.Items = append(res.Items, ItemResult{ID: id, Status: StatusCreated})
			}
		}
	}
	return idByName, nil
}

// migrateAsstToolAttachments attaches the already-migrated Tools that a
// Twilio assistant has to the corresponding new VoiceML assistant.
func migrateAsstToolAttachments(ctx context.Context, src assistantsSource, dst assistantsDest, twilioAssistantID, assistantLabel, newAssistantID string, toolIDByName map[string]string, res *Result) error {
	tools, err := src.ListToolsByAssistant(twilioAssistantID, &twasst.ListToolsByAssistantParams{})
	if err != nil {
		return fmt.Errorf("list Twilio tools for assistant %s: %w", assistantLabel, err)
	}
	existing, err := dst.ListAssistantTools(ctx, newAssistantID, voiceml.V1PageParams{})
	if err != nil {
		return fmt.Errorf("list VoiceML assistant tools: %w", err)
	}
	attached := make(map[string]bool, len(existing.Tools))
	for _, t := range existing.Tools {
		attached[deref(t.ID)] = true
	}

	for _, t := range tools {
		id := "tool-attachment " + t.Name + " (" + assistantLabel + ")"
		newToolID, ok := toolIDByName[t.Name]

		switch {
		case !ok:
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusFailed, Detail: "no migrated tool named " + t.Name})
		case attached[newToolID]:
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusSkipped, Detail: "already attached on VoiceML"})
		default:
			if aErr := dst.AttachToolToAssistant(ctx, newAssistantID, newToolID); aErr != nil {
				res.Items = append(res.Items, ItemResult{ID: id, Status: StatusFailed, Detail: aErr.Error()})
			} else {
				res.Items = append(res.Items, ItemResult{ID: id, Status: StatusCreated})
			}
		}
	}
	return nil
}

// migrateAsstKnowledgeAttachments attaches the already-migrated Knowledge
// sources that a Twilio assistant has to the corresponding new VoiceML
// assistant.
func migrateAsstKnowledgeAttachments(ctx context.Context, src assistantsSource, dst assistantsDest, twilioAssistantID, assistantLabel, newAssistantID string, knowledgeIDByName map[string]string, res *Result) error {
	sources, err := src.ListKnowledgeByAssistant(twilioAssistantID, &twasst.ListKnowledgeByAssistantParams{})
	if err != nil {
		return fmt.Errorf("list Twilio knowledge for assistant %s: %w", assistantLabel, err)
	}
	existing, err := dst.ListAssistantKnowledge(ctx, newAssistantID, voiceml.V1PageParams{})
	if err != nil {
		return fmt.Errorf("list VoiceML assistant knowledge: %w", err)
	}
	attached := make(map[string]bool, len(existing.Knowledge))
	for _, k := range existing.Knowledge {
		attached[deref(k.ID)] = true
	}

	for _, k := range sources {
		id := "knowledge-attachment " + k.Name + " (" + assistantLabel + ")"
		newKnowledgeID, ok := knowledgeIDByName[k.Name]

		switch {
		case !ok:
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusFailed, Detail: "no migrated knowledge named " + k.Name})
		case attached[newKnowledgeID]:
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusSkipped, Detail: "already attached on VoiceML"})
		default:
			if aErr := dst.AttachKnowledgeToAssistant(ctx, newAssistantID, newKnowledgeID); aErr != nil {
				res.Items = append(res.Items, ItemResult{ID: id, Status: StatusFailed, Detail: aErr.Error()})
			} else {
				res.Items = append(res.Items, ItemResult{ID: id, Status: StatusCreated})
			}
		}
	}
	return nil
}
