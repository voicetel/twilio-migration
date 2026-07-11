package migrate

import (
	"context"
	"fmt"

	twconv "github.com/twilio/twilio-go/rest/conversations/v1"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

// conversationsSource is the slice of twilio-go used to read the
// Conversations product's configuration surface.
type conversationsSource interface {
	ListService(params *twconv.ListServiceParams) ([]twconv.ConversationsV1Service, error)
	ListRole(params *twconv.ListRoleParams) ([]twconv.ConversationsV1Role, error)
	ListUser(params *twconv.ListUserParams) ([]twconv.ConversationsV1User, error)
	ListConversation(params *twconv.ListConversationParams) ([]twconv.ConversationsV1Conversation, error)
	ListConversationParticipant(conversationSid string, params *twconv.ListConversationParticipantParams) ([]twconv.ConversationsV1ConversationParticipant, error)
	ListConversationMessage(conversationSid string, params *twconv.ListConversationMessageParams) ([]twconv.ConversationsV1ConversationMessage, error)
	ListConversationScopedWebhook(conversationSid string, params *twconv.ListConversationScopedWebhookParams) ([]twconv.ConversationsV1ConversationScopedWebhook, error)
	ListConfigurationAddress(params *twconv.ListConfigurationAddressParams) ([]twconv.ConversationsV1ConfigurationAddress, error)
	FetchConfiguration() (*twconv.ConversationsV1Configuration, error)
	FetchConfigurationWebhook() (*twconv.ConversationsV1ConfigurationWebhook, error)
}

// conversationsDest is the slice of voiceml-go-sdk used to read/write the
// Conversations product's configuration surface.
type conversationsDest interface {
	ListServices(ctx context.Context, params voiceml.V1PageParams) (*voiceml.ConversationsV1ChatServiceList, error)
	CreateService(ctx context.Context, params voiceml.CreateServiceRequest) (*voiceml.ConversationsV1ChatService, error)
	ListRoles(ctx context.Context, params voiceml.V1PageParams) (*voiceml.ConversationsV1RoleList, error)
	CreateRole(ctx context.Context, params voiceml.CreateRoleRequest) (*voiceml.ConversationsV1Role, error)
	ListUsers(ctx context.Context, params voiceml.V1PageParams) (*voiceml.ConversationsV1UserList, error)
	CreateUser(ctx context.Context, params voiceml.CreateUserRequest) (*voiceml.ConversationsV1User, error)
	ListConversations(ctx context.Context, params voiceml.V1PageParams) (*voiceml.ConversationsV1ConversationList, error)
	CreateConversation(ctx context.Context, params voiceml.CreateConversationRequest) (*voiceml.ConversationsV1Conversation, error)
	ListParticipants(ctx context.Context, conversationSid string, params voiceml.V1PageParams) (*voiceml.ConversationsV1ConversationParticipantList, error)
	CreateParticipant(ctx context.Context, conversationSid string, params voiceml.CreateParticipantRequest) (*voiceml.ConversationsV1ConversationParticipant, error)
	ListMessages(ctx context.Context, conversationSid string, params voiceml.V1PageParams) (*voiceml.ConversationsV1ConversationMessageList, error)
	CreateMessage(ctx context.Context, conversationSid string, params voiceml.CreateMessageRequest) (*voiceml.ConversationsV1ConversationMessage, error)
	ListScopedWebhooks(ctx context.Context, conversationSid string, params voiceml.V1PageParams) (*voiceml.ConversationsV1ConversationScopedWebhookList, error)
	CreateScopedWebhook(ctx context.Context, conversationSid string, params voiceml.CreateScopedWebhookRequest) (*voiceml.ConversationsV1ConversationScopedWebhook, error)
	ListConfigAddresses(ctx context.Context, params voiceml.V1PageParams) (*voiceml.ConversationsV1ConfigAddressList, error)
	CreateConfigAddress(ctx context.Context, params voiceml.CreateConfigAddressRequest) (*voiceml.ConversationsV1ConfigAddress, error)
	FetchConfiguration(ctx context.Context) (*voiceml.ConversationsV1Configuration, error)
	UpdateConfiguration(ctx context.Context, params voiceml.UpdateConfigurationRequest) (*voiceml.ConversationsV1Configuration, error)
	FetchConfigurationWebhook(ctx context.Context) (*voiceml.ConversationsV1ConfigurationWebhook, error)
	UpdateConfigurationWebhook(ctx context.Context, params voiceml.UpdateConfigurationWebhookRequest) (*voiceml.ConversationsV1ConfigurationWebhook, error)
}

// Conversations migrates the Conversations product's CONFIGURATION surface:
// Services (as records — their own nested Roles/Users/Conversations are NOT
// migrated, see below), the default (unscoped) account's Roles, Users,
// Conversations (+ Participants, Messages, scoped Webhooks), account-level
// Config Addresses, and the account Configuration / ConfigurationWebhook
// singletons.
//
// Deliberately out of scope:
//   - Each named Service's OWN nested Roles/Users/Conversations/etc.
//     (Services/{sid}/...) — only the default/unscoped equivalents are
//     migrated, plus the Service records themselves. Doubling every
//     sub-resource migration per-service was judged out of scope for this
//     pass (owner decision).
//   - Credentials (push notification certs/keys, APN/GCM/FCM): Twilio's read
//     side (ConversationsV1Credential) never returns the secret material
//     (Certificate/PrivateKey/ApiKey/Secret) that create requires — there is
//     nothing to copy. Unlike SIP passwords, VoiceML cannot mint a valid
//     substitute for an Apple/Google-issued credential, so this sub-resource
//     cannot be migrated at all.
//   - MessagingServiceSid cross-refs (on Conversations and the account
//     Configuration) are left unset rather than bridged to an already-
//     migrated Messaging Service — a deliberate simplification, since the
//     field is optional and functionally secondary to the conversation/
//     account itself.
//   - Message event filters/triggers on scoped Webhooks: VoiceML's
//     CreateScopedWebhookRequest has no Filters/Triggers fields (only
//     URL/Method/FlowSid/ReplayAfter) — an SDK capability gap, not a choice
//     made here.
type Conversations struct{}

// Name implements Migrator.
func (Conversations) Name() string { return "conversations" }

// Migrate implements Migrator.
func (Conversations) Migrate(ctx context.Context, c *Clients, opts Options) (Result, error) {
	return migrateConversations(ctx, c.TwilioConversations, c.VoiceML.ConversationsV1, opts)
}

func migrateConversations(ctx context.Context, src conversationsSource, dst conversationsDest, opts Options) (Result, error) {
	res := Result{Resource: "conversations"}

	if err := migrateConvServices(ctx, src, dst, opts, &res); err != nil {
		return res, err
	}
	roleSIDByOldSID, err := migrateConvRoles(ctx, src, dst, opts, &res)
	if err != nil {
		return res, err
	}
	if err := migrateConvUsers(ctx, src, dst, opts, roleSIDByOldSID, &res); err != nil {
		return res, err
	}
	if err := migrateConvConversations(ctx, src, dst, opts, roleSIDByOldSID, &res); err != nil {
		return res, err
	}
	if err := migrateConvConfigAddresses(ctx, src, dst, opts, &res); err != nil {
		return res, err
	}
	if err := migrateConvConfiguration(ctx, src, dst, opts, &res); err != nil {
		return res, err
	}

	return res, nil
}

// migrateConvServices migrates Conversation Services, idempotent by
// friendly name.
func migrateConvServices(ctx context.Context, src conversationsSource, dst conversationsDest, opts Options, res *Result) error {
	services, err := src.ListService(&twconv.ListServiceParams{})
	if err != nil {
		return fmt.Errorf("list Twilio conversation services: %w", err)
	}
	existing, err := dst.ListServices(ctx, voiceml.V1PageParams{})
	if err != nil {
		return fmt.Errorf("list VoiceML conversation services: %w", err)
	}
	have := make(map[string]bool, len(existing.Services))
	for _, s := range existing.Services {
		have[deref(s.FriendlyName)] = true
	}

	for _, s := range services {
		name := deref(s.FriendlyName)
		id := "service " + name

		switch {
		case name == "":
			res.Items = append(res.Items, ItemResult{ID: "service", Status: StatusFailed, Detail: "source service has no friendly_name"})
		case have[name]:
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusSkipped, Detail: "already present on VoiceML"})
		case opts.DryRun:
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusPlanned})
		default:
			if _, cErr := dst.CreateService(ctx, voiceml.CreateServiceRequest{FriendlyName: name}); cErr != nil {
				res.Items = append(res.Items, ItemResult{ID: id, Status: StatusFailed, Detail: cErr.Error()})
			} else {
				res.Items = append(res.Items, ItemResult{ID: id, Status: StatusCreated})
			}
		}
	}
	return nil
}

// migrateConvRoles migrates the default (unscoped) account's Roles,
// idempotent by friendly name. Returns a friendly-name→new-SID map so Users
// and Participants can resolve an optional RoleSid reference.
func migrateConvRoles(ctx context.Context, src conversationsSource, dst conversationsDest, opts Options, res *Result) (map[string]string, error) {
	roles, err := src.ListRole(&twconv.ListRoleParams{})
	if err != nil {
		return nil, fmt.Errorf("list Twilio conversation roles: %w", err)
	}
	existing, err := dst.ListRoles(ctx, voiceml.V1PageParams{})
	if err != nil {
		return nil, fmt.Errorf("list VoiceML conversation roles: %w", err)
	}
	sidByName := make(map[string]string, len(existing.Roles))
	for _, r := range existing.Roles {
		sidByName[deref(r.FriendlyName)] = deref(r.Sid)
	}

	// roleSIDByOldSID bridges a Twilio role SID (what User.RoleSid /
	// Participant.RoleSid actually reference) to the new VoiceML role SID,
	// via the friendly name the two accounts' roles share.
	roleSIDByOldSID := make(map[string]string, len(roles))

	for _, r := range roles {
		name := deref(r.FriendlyName)
		oldSID := deref(r.Sid)
		id := "role " + name

		switch {
		case name == "":
			res.Items = append(res.Items, ItemResult{ID: "role", Status: StatusFailed, Detail: "source role has no friendly_name"})
			continue
		case sidByName[name] != "":
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusSkipped, Detail: "already present on VoiceML"})
		case opts.DryRun:
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusPlanned})
			continue
		default:
			var permissions []string
			if r.Permissions != nil {
				permissions = *r.Permissions
			}
			created, cErr := dst.CreateRole(ctx, voiceml.CreateRoleRequest{
				FriendlyName: name,
				Type:         deref(r.Type),
				Permission:   permissions,
			})
			if cErr != nil {
				res.Items = append(res.Items, ItemResult{ID: id, Status: StatusFailed, Detail: cErr.Error()})
				continue
			}
			sidByName[name] = deref(created.Sid)
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusCreated})
		}
		if oldSID != "" {
			roleSIDByOldSID[oldSID] = sidByName[name]
		}
	}
	return roleSIDByOldSID, nil
}

// migrateConvUsers migrates the default (unscoped) account's Users,
// idempotent by Identity. RoleSid is bridged via roleSIDByOldSID when
// resolvable, left unset otherwise (optional field).
func migrateConvUsers(ctx context.Context, src conversationsSource, dst conversationsDest, opts Options, roleSIDByOldSID map[string]string, res *Result) error {
	users, err := src.ListUser(&twconv.ListUserParams{})
	if err != nil {
		return fmt.Errorf("list Twilio conversation users: %w", err)
	}
	existing, err := dst.ListUsers(ctx, voiceml.V1PageParams{})
	if err != nil {
		return fmt.Errorf("list VoiceML conversation users: %w", err)
	}
	have := make(map[string]bool, len(existing.Users))
	for _, u := range existing.Users {
		have[deref(u.Identity)] = true
	}

	for _, u := range users {
		identity := deref(u.Identity)
		id := "user " + identity

		switch {
		case identity == "":
			res.Items = append(res.Items, ItemResult{ID: "user", Status: StatusFailed, Detail: "source user has no identity"})
		case have[identity]:
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusSkipped, Detail: "already present on VoiceML"})
		case opts.DryRun:
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusPlanned})
		default:
			params := voiceml.CreateUserRequest{
				Identity:     identity,
				FriendlyName: u.FriendlyName,
				Attributes:   u.Attributes,
			}
			if newRoleSID, ok := roleSIDByOldSID[deref(u.RoleSid)]; ok {
				params.RoleSid = &newRoleSID
			}
			if _, cErr := dst.CreateUser(ctx, params); cErr != nil {
				res.Items = append(res.Items, ItemResult{ID: id, Status: StatusFailed, Detail: cErr.Error()})
			} else {
				res.Items = append(res.Items, ItemResult{ID: id, Status: StatusCreated})
			}
		}
	}
	return nil
}

// convKey returns a Conversation's natural idempotency key: UniqueName if
// set, else FriendlyName, else "".
func convKey(c twconv.ConversationsV1Conversation) string {
	if k := deref(c.UniqueName); k != "" {
		return k
	}
	return deref(c.FriendlyName)
}

// migrateConvConversations migrates the default (unscoped) account's
// Conversations, idempotent by UniqueName (falling back to FriendlyName),
// then cascades into each conversation's Participants, Messages, and scoped
// Webhooks.
func migrateConvConversations(ctx context.Context, src conversationsSource, dst conversationsDest, opts Options, roleSIDByOldSID map[string]string, res *Result) error {
	conversations, err := src.ListConversation(&twconv.ListConversationParams{})
	if err != nil {
		return fmt.Errorf("list Twilio conversations: %w", err)
	}
	existing, err := dst.ListConversations(ctx, voiceml.V1PageParams{})
	if err != nil {
		return fmt.Errorf("list VoiceML conversations: %w", err)
	}
	sidByKey := make(map[string]string, len(existing.Conversations))
	for _, c := range existing.Conversations {
		key := deref(c.UniqueName)
		if key == "" {
			key = deref(c.FriendlyName)
		}
		if key != "" {
			sidByKey[key] = deref(c.Sid)
		}
	}

	for _, c := range conversations {
		key := convKey(c)
		id := "conversation " + key
		var newSID string

		switch {
		case key == "":
			res.Items = append(res.Items, ItemResult{ID: "conversation", Status: StatusFailed, Detail: "source conversation has no unique_name or friendly_name"})
			continue
		case sidByKey[key] != "":
			newSID = sidByKey[key]
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusSkipped, Detail: "already present on VoiceML"})
		case opts.DryRun:
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusPlanned})
		default:
			created, cErr := dst.CreateConversation(ctx, voiceml.CreateConversationRequest{
				FriendlyName: c.FriendlyName,
				UniqueName:   c.UniqueName,
				Attributes:   c.Attributes,
			})
			if cErr != nil {
				res.Items = append(res.Items, ItemResult{ID: id, Status: StatusFailed, Detail: cErr.Error()})
				continue
			}
			newSID = deref(created.Sid)
			sidByKey[key] = newSID
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusCreated})
		}

		if newSID == "" {
			// Dry-run: no new/existing SID to cascade into.
			continue
		}
		if err := migrateConvParticipants(ctx, src, dst, opts, roleSIDByOldSID, deref(c.Sid), key, newSID, res); err != nil {
			return err
		}
		if err := migrateConvMessages(ctx, src, dst, opts, deref(c.Sid), key, newSID, res); err != nil {
			return err
		}
		if err := migrateConvWebhooks(ctx, src, dst, opts, deref(c.Sid), key, newSID, res); err != nil {
			return err
		}
	}
	return nil
}

// participantKey returns a Participant's natural idempotency key: Identity
// if set, else the messaging-binding address, else "".
func participantKey(p twconv.ConversationsV1ConversationParticipant) string {
	if k := deref(p.Identity); k != "" {
		return k
	}
	if p.MessagingBinding == nil {
		return ""
	}
	binding, ok := (*p.MessagingBinding).(map[string]interface{})
	if !ok {
		return ""
	}
	addr, _ := binding["address"].(string)
	return addr
}

// webhookConfigFields extracts the url/method/flow_sid fields from a scoped
// webhook's target-dependent Configuration blob (an untyped JSON object —
// its shape varies by Target: "webhook"/"trigger" carry url+method,
// "studio" carries flow_sid). Any field absent for the target in question
// comes back nil.
func webhookConfigFields(cfg *interface{}) (url, method, flowSID *string) {
	if cfg == nil {
		return nil, nil, nil
	}
	m, ok := (*cfg).(map[string]interface{})
	if !ok {
		return nil, nil, nil
	}
	if v, ok := m["url"].(string); ok {
		url = &v
	}
	if v, ok := m["method"].(string); ok {
		method = &v
	}
	if v, ok := m["flow_sid"].(string); ok {
		flowSID = &v
	}
	return url, method, flowSID
}

func migrateConvParticipants(ctx context.Context, src conversationsSource, dst conversationsDest, opts Options, roleSIDByOldSID map[string]string, twilioConvSID, convLabel, newConvSID string, res *Result) error {
	participants, err := src.ListConversationParticipant(twilioConvSID, &twconv.ListConversationParticipantParams{})
	if err != nil {
		return fmt.Errorf("list Twilio participants for conversation %s: %w", convLabel, err)
	}
	existing, err := dst.ListParticipants(ctx, newConvSID, voiceml.V1PageParams{})
	if err != nil {
		return fmt.Errorf("list VoiceML participants: %w", err)
	}
	have := make(map[string]bool, len(existing.Participants))
	for _, p := range existing.Participants {
		key := deref(p.Identity)
		if key == "" && p.MessagingBinding != nil {
			key = p.MessagingBinding["address"]
		}
		if key != "" {
			have[key] = true
		}
	}

	for _, p := range participants {
		key := participantKey(p)
		id := "participant " + key + " (" + convLabel + ")"

		switch {
		case key == "":
			res.Items = append(res.Items, ItemResult{ID: "participant (" + convLabel + ")", Status: StatusFailed, Detail: "source participant has no identity or messaging-binding address"})
		case have[key]:
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusSkipped, Detail: "already present on VoiceML"})
		case opts.DryRun:
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusPlanned})
		default:
			params := voiceml.CreateParticipantRequest{
				Identity:   p.Identity,
				Attributes: p.Attributes,
			}
			if newRoleSID, ok := roleSIDByOldSID[deref(p.RoleSid)]; ok {
				params.RoleSid = &newRoleSID
			}
			if _, cErr := dst.CreateParticipant(ctx, newConvSID, params); cErr != nil {
				res.Items = append(res.Items, ItemResult{ID: id, Status: StatusFailed, Detail: cErr.Error()})
			} else {
				res.Items = append(res.Items, ItemResult{ID: id, Status: StatusCreated})
			}
		}
	}
	return nil
}

// migrateConvMessages migrates a conversation's Messages. Twilio message
// SIDs are opaque and there is no other stable per-message natural key, so
// idempotency is a coarse conversation-level count check: if VoiceML already
// has at least as many messages as Twilio for this conversation, the whole
// batch is treated as already migrated (one Skipped item); otherwise every
// Twilio message is created in order (one item each).
func migrateConvMessages(ctx context.Context, src conversationsSource, dst conversationsDest, opts Options, twilioConvSID, convLabel, newConvSID string, res *Result) error {
	messages, err := src.ListConversationMessage(twilioConvSID, &twconv.ListConversationMessageParams{})
	if err != nil {
		return fmt.Errorf("list Twilio messages for conversation %s: %w", convLabel, err)
	}
	existing, err := dst.ListMessages(ctx, newConvSID, voiceml.V1PageParams{})
	if err != nil {
		return fmt.Errorf("list VoiceML messages: %w", err)
	}

	id := "messages (" + convLabel + ")"
	switch {
	case len(existing.Messages) >= len(messages):
		res.Items = append(res.Items, ItemResult{ID: id, Status: StatusSkipped, Detail: fmt.Sprintf("VoiceML already has %d/%d messages", len(existing.Messages), len(messages))})
		return nil
	case opts.DryRun:
		res.Items = append(res.Items, ItemResult{ID: id, Status: StatusPlanned, Detail: fmt.Sprintf("%d message(s) would be created", len(messages))})
		return nil
	}

	for i, m := range messages {
		mid := fmt.Sprintf("message %d (%s)", i, convLabel)
		if _, cErr := dst.CreateMessage(ctx, newConvSID, voiceml.CreateMessageRequest{
			Author:     m.Author,
			Body:       m.Body,
			Attributes: m.Attributes,
		}); cErr != nil {
			res.Items = append(res.Items, ItemResult{ID: mid, Status: StatusFailed, Detail: cErr.Error()})
		} else {
			res.Items = append(res.Items, ItemResult{ID: mid, Status: StatusCreated})
		}
	}
	return nil
}

// migrateConvWebhooks migrates a conversation's scoped Webhooks, idempotent
// by Target. VoiceML's CreateScopedWebhookRequest has no Filters/Triggers
// fields (an SDK capability gap), so only URL/Method/FlowSid carry over.
func migrateConvWebhooks(ctx context.Context, src conversationsSource, dst conversationsDest, opts Options, twilioConvSID, convLabel, newConvSID string, res *Result) error {
	webhooks, err := src.ListConversationScopedWebhook(twilioConvSID, &twconv.ListConversationScopedWebhookParams{})
	if err != nil {
		return fmt.Errorf("list Twilio webhooks for conversation %s: %w", convLabel, err)
	}
	existing, err := dst.ListScopedWebhooks(ctx, newConvSID, voiceml.V1PageParams{})
	if err != nil {
		return fmt.Errorf("list VoiceML webhooks: %w", err)
	}
	have := make(map[string]bool, len(existing.Webhooks))
	for _, w := range existing.Webhooks {
		have[deref(w.Target)] = true
	}

	for _, w := range webhooks {
		target := deref(w.Target)
		id := "webhook " + target + " (" + convLabel + ")"

		switch {
		case target == "":
			res.Items = append(res.Items, ItemResult{ID: "webhook (" + convLabel + ")", Status: StatusFailed, Detail: "source webhook has no target"})
		case have[target]:
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusSkipped, Detail: "already present on VoiceML"})
		case opts.DryRun:
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusPlanned})
		default:
			cfgURL, cfgMethod, cfgFlowSid := webhookConfigFields(w.Configuration)
			if _, cErr := dst.CreateScopedWebhook(ctx, newConvSID, voiceml.CreateScopedWebhookRequest{
				Target:               target,
				ConfigurationURL:     cfgURL,
				ConfigurationMethod:  cfgMethod,
				ConfigurationFlowSid: cfgFlowSid,
			}); cErr != nil {
				res.Items = append(res.Items, ItemResult{ID: id, Status: StatusFailed, Detail: cErr.Error()})
			} else {
				res.Items = append(res.Items, ItemResult{ID: id, Status: StatusCreated})
			}
		}
	}
	return nil
}

// migrateConvConfigAddresses migrates account-level Config Addresses,
// idempotent by (Type, Address).
func migrateConvConfigAddresses(ctx context.Context, src conversationsSource, dst conversationsDest, opts Options, res *Result) error {
	addresses, err := src.ListConfigurationAddress(&twconv.ListConfigurationAddressParams{})
	if err != nil {
		return fmt.Errorf("list Twilio config addresses: %w", err)
	}
	existing, err := dst.ListConfigAddresses(ctx, voiceml.V1PageParams{})
	if err != nil {
		return fmt.Errorf("list VoiceML config addresses: %w", err)
	}
	have := make(map[string]bool, len(existing.Addresses))
	for _, a := range existing.Addresses {
		have[deref(a.Type)+"|"+deref(a.Address)] = true
	}

	for _, a := range addresses {
		typ := deref(a.Type)
		addr := deref(a.Address)
		key := typ + "|" + addr
		id := "config-address " + addr + " (" + typ + ")"

		switch {
		case addr == "" || typ == "":
			res.Items = append(res.Items, ItemResult{ID: "config-address", Status: StatusFailed, Detail: "source config address has no type or address"})
		case have[key]:
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusSkipped, Detail: "already present on VoiceML"})
		case opts.DryRun:
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusPlanned})
		default:
			if _, cErr := dst.CreateConfigAddress(ctx, voiceml.CreateConfigAddressRequest{
				Type:         typ,
				Address:      addr,
				FriendlyName: a.FriendlyName,
			}); cErr != nil {
				res.Items = append(res.Items, ItemResult{ID: id, Status: StatusFailed, Detail: cErr.Error()})
			} else {
				res.Items = append(res.Items, ItemResult{ID: id, Status: StatusCreated})
			}
		}
	}
	return nil
}

// migrateConvConfiguration syncs the account-level Configuration and
// ConfigurationWebhook singletons. There is no "already present" concept for
// a singleton — every run re-applies Twilio's current values (skipped
// entirely under DryRun, like every other write in this tool).
func migrateConvConfiguration(ctx context.Context, src conversationsSource, dst conversationsDest, opts Options, res *Result) error {
	cfg, err := src.FetchConfiguration()
	if err != nil {
		return fmt.Errorf("fetch Twilio configuration: %w", err)
	}
	webhookCfg, err := src.FetchConfigurationWebhook()
	if err != nil {
		return fmt.Errorf("fetch Twilio configuration webhook: %w", err)
	}

	if opts.DryRun {
		res.Items = append(res.Items,
			ItemResult{ID: "configuration", Status: StatusPlanned},
			ItemResult{ID: "configuration-webhook", Status: StatusPlanned},
		)
		return nil
	}

	if _, cErr := dst.UpdateConfiguration(ctx, voiceml.UpdateConfigurationRequest{
		DefaultInactiveTimer: cfg.DefaultInactiveTimer,
		DefaultClosedTimer:   cfg.DefaultClosedTimer,
	}); cErr != nil {
		res.Items = append(res.Items, ItemResult{ID: "configuration", Status: StatusFailed, Detail: cErr.Error()})
	} else {
		res.Items = append(res.Items, ItemResult{ID: "configuration", Status: StatusCreated})
	}

	var filters []string
	if webhookCfg.Filters != nil {
		filters = *webhookCfg.Filters
	}
	if _, cErr := dst.UpdateConfigurationWebhook(ctx, voiceml.UpdateConfigurationWebhookRequest{
		Method:         webhookCfg.Method,
		Filters:        filters,
		PreWebhookURL:  webhookCfg.PreWebhookUrl,
		PostWebhookURL: webhookCfg.PostWebhookUrl,
		Target:         webhookCfg.Target,
	}); cErr != nil {
		res.Items = append(res.Items, ItemResult{ID: "configuration-webhook", Status: StatusFailed, Detail: cErr.Error()})
	} else {
		res.Items = append(res.Items, ItemResult{ID: "configuration-webhook", Status: StatusCreated})
	}

	return nil
}
