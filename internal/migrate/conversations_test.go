package migrate

import (
	"context"
	"errors"
	"testing"

	twconv "github.com/twilio/twilio-go/rest/conversations/v1"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

// --- fakes ---

type fakeConvSource struct {
	services      []twconv.ConversationsV1Service
	roles         []twconv.ConversationsV1Role
	users         []twconv.ConversationsV1User
	conversations []twconv.ConversationsV1Conversation
	participants  map[string][]twconv.ConversationsV1ConversationParticipant
	messages      map[string][]twconv.ConversationsV1ConversationMessage
	webhooks      map[string][]twconv.ConversationsV1ConversationScopedWebhook
	configAddrs   []twconv.ConversationsV1ConfigurationAddress
	configuration *twconv.ConversationsV1Configuration
	configWebhook *twconv.ConversationsV1ConfigurationWebhook

	serviceErr, roleErr, userErr, conversationErr, participantErr,
	messageErr, webhookErr, configAddrErr, configErr, configWebhookErr error
}

func (f fakeConvSource) ListService(*twconv.ListServiceParams) ([]twconv.ConversationsV1Service, error) {
	if f.serviceErr != nil {
		return nil, f.serviceErr
	}
	return f.services, nil
}
func (f fakeConvSource) ListRole(*twconv.ListRoleParams) ([]twconv.ConversationsV1Role, error) {
	if f.roleErr != nil {
		return nil, f.roleErr
	}
	return f.roles, nil
}
func (f fakeConvSource) ListUser(*twconv.ListUserParams) ([]twconv.ConversationsV1User, error) {
	if f.userErr != nil {
		return nil, f.userErr
	}
	return f.users, nil
}
func (f fakeConvSource) ListConversation(*twconv.ListConversationParams) ([]twconv.ConversationsV1Conversation, error) {
	if f.conversationErr != nil {
		return nil, f.conversationErr
	}
	return f.conversations, nil
}
func (f fakeConvSource) ListConversationParticipant(sid string, _ *twconv.ListConversationParticipantParams) ([]twconv.ConversationsV1ConversationParticipant, error) {
	if f.participantErr != nil {
		return nil, f.participantErr
	}
	return f.participants[sid], nil
}
func (f fakeConvSource) ListConversationMessage(sid string, _ *twconv.ListConversationMessageParams) ([]twconv.ConversationsV1ConversationMessage, error) {
	if f.messageErr != nil {
		return nil, f.messageErr
	}
	return f.messages[sid], nil
}
func (f fakeConvSource) ListConversationScopedWebhook(sid string, _ *twconv.ListConversationScopedWebhookParams) ([]twconv.ConversationsV1ConversationScopedWebhook, error) {
	if f.webhookErr != nil {
		return nil, f.webhookErr
	}
	return f.webhooks[sid], nil
}
func (f fakeConvSource) ListConfigurationAddress(*twconv.ListConfigurationAddressParams) ([]twconv.ConversationsV1ConfigurationAddress, error) {
	if f.configAddrErr != nil {
		return nil, f.configAddrErr
	}
	return f.configAddrs, nil
}
func (f fakeConvSource) FetchConfiguration() (*twconv.ConversationsV1Configuration, error) {
	if f.configErr != nil {
		return nil, f.configErr
	}
	if f.configuration != nil {
		return f.configuration, nil
	}
	return &twconv.ConversationsV1Configuration{}, nil
}
func (f fakeConvSource) FetchConfigurationWebhook() (*twconv.ConversationsV1ConfigurationWebhook, error) {
	if f.configWebhookErr != nil {
		return nil, f.configWebhookErr
	}
	if f.configWebhook != nil {
		return f.configWebhook, nil
	}
	return &twconv.ConversationsV1ConfigurationWebhook{}, nil
}

type fakeConvDest struct {
	existingServices      []voiceml.ConversationsV1ChatService
	existingRoles         []voiceml.ConversationsV1Role
	existingUsers         []voiceml.ConversationsV1User
	existingConversations []voiceml.ConversationsV1Conversation
	existingParticipants  map[string][]voiceml.ConversationsV1ConversationParticipant
	existingMessages      map[string][]voiceml.ConversationsV1ConversationMessage
	existingWebhooks      map[string][]voiceml.ConversationsV1ConversationScopedWebhook
	existingConfigAddrs   []voiceml.ConversationsV1ConfigAddress

	createdServices      []voiceml.CreateServiceRequest
	createdRoles         []voiceml.CreateRoleRequest
	createdUsers         []voiceml.CreateUserRequest
	createdConversations []voiceml.CreateConversationRequest
	createdParticipants  map[string][]voiceml.CreateParticipantRequest
	createdMessages      map[string][]voiceml.CreateMessageRequest
	createdWebhooks      map[string][]voiceml.CreateScopedWebhookRequest
	createdConfigAddrs   []voiceml.CreateConfigAddressRequest
	updatedConfig        *voiceml.UpdateConfigurationRequest
	updatedConfigWebhook *voiceml.UpdateConfigurationWebhookRequest

	listServiceErr, createServiceErr,
	listRoleErr, createRoleErr,
	listUserErr, createUserErr,
	listConvErr, createConvErr,
	listParticipantErr, createParticipantErr,
	listMessageErr, createMessageErr,
	listWebhookErr, createWebhookErr,
	listConfigAddrErr, createConfigAddrErr,
	fetchConfigErr, updateConfigErr,
	fetchConfigWebhookErr, updateConfigWebhookErr error
}

func (f *fakeConvDest) ListServices(context.Context, voiceml.V1PageParams) (*voiceml.ConversationsV1ChatServiceList, error) {
	if f.listServiceErr != nil {
		return nil, f.listServiceErr
	}
	return &voiceml.ConversationsV1ChatServiceList{Services: f.existingServices}, nil
}
func (f *fakeConvDest) CreateService(_ context.Context, p voiceml.CreateServiceRequest) (*voiceml.ConversationsV1ChatService, error) {
	if f.createServiceErr != nil {
		return nil, f.createServiceErr
	}
	f.createdServices = append(f.createdServices, p)
	return &voiceml.ConversationsV1ChatService{Sid: strp("CHnew"), FriendlyName: &p.FriendlyName}, nil
}
func (f *fakeConvDest) ListRoles(context.Context, voiceml.V1PageParams) (*voiceml.ConversationsV1RoleList, error) {
	if f.listRoleErr != nil {
		return nil, f.listRoleErr
	}
	return &voiceml.ConversationsV1RoleList{Roles: f.existingRoles}, nil
}
func (f *fakeConvDest) CreateRole(_ context.Context, p voiceml.CreateRoleRequest) (*voiceml.ConversationsV1Role, error) {
	if f.createRoleErr != nil {
		return nil, f.createRoleErr
	}
	f.createdRoles = append(f.createdRoles, p)
	return &voiceml.ConversationsV1Role{Sid: strp("RLnew"), FriendlyName: &p.FriendlyName}, nil
}
func (f *fakeConvDest) ListUsers(context.Context, voiceml.V1PageParams) (*voiceml.ConversationsV1UserList, error) {
	if f.listUserErr != nil {
		return nil, f.listUserErr
	}
	return &voiceml.ConversationsV1UserList{Users: f.existingUsers}, nil
}
func (f *fakeConvDest) CreateUser(_ context.Context, p voiceml.CreateUserRequest) (*voiceml.ConversationsV1User, error) {
	if f.createUserErr != nil {
		return nil, f.createUserErr
	}
	f.createdUsers = append(f.createdUsers, p)
	return &voiceml.ConversationsV1User{Sid: strp("USnew"), Identity: &p.Identity}, nil
}
func (f *fakeConvDest) ListConversations(context.Context, voiceml.V1PageParams) (*voiceml.ConversationsV1ConversationList, error) {
	if f.listConvErr != nil {
		return nil, f.listConvErr
	}
	return &voiceml.ConversationsV1ConversationList{Conversations: f.existingConversations}, nil
}
func (f *fakeConvDest) CreateConversation(_ context.Context, p voiceml.CreateConversationRequest) (*voiceml.ConversationsV1Conversation, error) {
	if f.createConvErr != nil {
		return nil, f.createConvErr
	}
	f.createdConversations = append(f.createdConversations, p)
	return &voiceml.ConversationsV1Conversation{Sid: strp("CHnewconv"), FriendlyName: p.FriendlyName, UniqueName: p.UniqueName}, nil
}
func (f *fakeConvDest) ListParticipants(_ context.Context, convSid string, _ voiceml.V1PageParams) (*voiceml.ConversationsV1ConversationParticipantList, error) {
	if f.listParticipantErr != nil {
		return nil, f.listParticipantErr
	}
	return &voiceml.ConversationsV1ConversationParticipantList{Participants: f.existingParticipants[convSid]}, nil
}
func (f *fakeConvDest) CreateParticipant(_ context.Context, convSid string, p voiceml.CreateParticipantRequest) (*voiceml.ConversationsV1ConversationParticipant, error) {
	if f.createParticipantErr != nil {
		return nil, f.createParticipantErr
	}
	if f.createdParticipants == nil {
		f.createdParticipants = map[string][]voiceml.CreateParticipantRequest{}
	}
	f.createdParticipants[convSid] = append(f.createdParticipants[convSid], p)
	return &voiceml.ConversationsV1ConversationParticipant{Sid: strp("PTnew")}, nil
}
func (f *fakeConvDest) ListMessages(_ context.Context, convSid string, _ voiceml.V1PageParams) (*voiceml.ConversationsV1ConversationMessageList, error) {
	if f.listMessageErr != nil {
		return nil, f.listMessageErr
	}
	return &voiceml.ConversationsV1ConversationMessageList{Messages: f.existingMessages[convSid]}, nil
}
func (f *fakeConvDest) CreateMessage(_ context.Context, convSid string, p voiceml.CreateMessageRequest) (*voiceml.ConversationsV1ConversationMessage, error) {
	if f.createMessageErr != nil {
		return nil, f.createMessageErr
	}
	if f.createdMessages == nil {
		f.createdMessages = map[string][]voiceml.CreateMessageRequest{}
	}
	f.createdMessages[convSid] = append(f.createdMessages[convSid], p)
	return &voiceml.ConversationsV1ConversationMessage{Sid: strp("MSnew")}, nil
}
func (f *fakeConvDest) ListScopedWebhooks(_ context.Context, convSid string, _ voiceml.V1PageParams) (*voiceml.ConversationsV1ConversationScopedWebhookList, error) {
	if f.listWebhookErr != nil {
		return nil, f.listWebhookErr
	}
	return &voiceml.ConversationsV1ConversationScopedWebhookList{Webhooks: f.existingWebhooks[convSid]}, nil
}
func (f *fakeConvDest) CreateScopedWebhook(_ context.Context, convSid string, p voiceml.CreateScopedWebhookRequest) (*voiceml.ConversationsV1ConversationScopedWebhook, error) {
	if f.createWebhookErr != nil {
		return nil, f.createWebhookErr
	}
	if f.createdWebhooks == nil {
		f.createdWebhooks = map[string][]voiceml.CreateScopedWebhookRequest{}
	}
	f.createdWebhooks[convSid] = append(f.createdWebhooks[convSid], p)
	return &voiceml.ConversationsV1ConversationScopedWebhook{Sid: strp("WHnew")}, nil
}
func (f *fakeConvDest) ListConfigAddresses(context.Context, voiceml.V1PageParams) (*voiceml.ConversationsV1ConfigAddressList, error) {
	if f.listConfigAddrErr != nil {
		return nil, f.listConfigAddrErr
	}
	return &voiceml.ConversationsV1ConfigAddressList{Addresses: f.existingConfigAddrs}, nil
}
func (f *fakeConvDest) CreateConfigAddress(_ context.Context, p voiceml.CreateConfigAddressRequest) (*voiceml.ConversationsV1ConfigAddress, error) {
	if f.createConfigAddrErr != nil {
		return nil, f.createConfigAddrErr
	}
	f.createdConfigAddrs = append(f.createdConfigAddrs, p)
	return &voiceml.ConversationsV1ConfigAddress{Sid: strp("IGnew")}, nil
}
func (f *fakeConvDest) FetchConfiguration(context.Context) (*voiceml.ConversationsV1Configuration, error) {
	if f.fetchConfigErr != nil {
		return nil, f.fetchConfigErr
	}
	return &voiceml.ConversationsV1Configuration{}, nil
}
func (f *fakeConvDest) UpdateConfiguration(_ context.Context, p voiceml.UpdateConfigurationRequest) (*voiceml.ConversationsV1Configuration, error) {
	if f.updateConfigErr != nil {
		return nil, f.updateConfigErr
	}
	f.updatedConfig = &p
	return &voiceml.ConversationsV1Configuration{}, nil
}
func (f *fakeConvDest) FetchConfigurationWebhook(context.Context) (*voiceml.ConversationsV1ConfigurationWebhook, error) {
	if f.fetchConfigWebhookErr != nil {
		return nil, f.fetchConfigWebhookErr
	}
	return &voiceml.ConversationsV1ConfigurationWebhook{}, nil
}
func (f *fakeConvDest) UpdateConfigurationWebhook(_ context.Context, p voiceml.UpdateConfigurationWebhookRequest) (*voiceml.ConversationsV1ConfigurationWebhook, error) {
	if f.updateConfigWebhookErr != nil {
		return nil, f.updateConfigWebhookErr
	}
	f.updatedConfigWebhook = &p
	return &voiceml.ConversationsV1ConfigurationWebhook{}, nil
}

// --- full success-path test ---

func fullConvSource() fakeConvSource {
	webBinding := interface{}(map[string]interface{}{"address": "+15551234567"})
	webhookCfg := interface{}(map[string]interface{}{"url": "https://example.com/hook", "method": "POST", "flow_sid": "FWxxx"})
	return fakeConvSource{
		services: []twconv.ConversationsV1Service{{Sid: strp("ISold"), FriendlyName: strp("Support")}},
		roles:    []twconv.ConversationsV1Role{{Sid: strp("RLold"), FriendlyName: strp("admin"), Type: strp("conversation"), Permissions: &[]string{"sendMessage"}}},
		users: []twconv.ConversationsV1User{
			{Identity: strp("alice"), FriendlyName: strp("Alice"), RoleSid: strp("RLold")},
		},
		conversations: []twconv.ConversationsV1Conversation{
			{Sid: strp("CHold1"), UniqueName: strp("support-thread")},
			{Sid: strp("CHold2"), FriendlyName: strp("No unique name")},
		},
		participants: map[string][]twconv.ConversationsV1ConversationParticipant{
			"CHold1": {
				{Identity: strp("alice"), RoleSid: strp("RLold")},
				{MessagingBinding: &webBinding},
			},
		},
		messages: map[string][]twconv.ConversationsV1ConversationMessage{
			"CHold1": {{Author: strp("alice"), Body: strp("hi")}},
		},
		webhooks: map[string][]twconv.ConversationsV1ConversationScopedWebhook{
			"CHold1": {{Target: strp("webhook"), Configuration: &webhookCfg}},
		},
		configAddrs: []twconv.ConversationsV1ConfigurationAddress{
			{Type: strp("sms"), Address: strp("+15559876543"), FriendlyName: strp("Main line")},
		},
		configuration: &twconv.ConversationsV1Configuration{
			DefaultInactiveTimer: strp("PT1H"),
			DefaultClosedTimer:   strp("PT24H"),
		},
		configWebhook: &twconv.ConversationsV1ConfigurationWebhook{
			Method:         strp("POST"),
			Filters:        &[]string{"onMessageAdded"},
			PreWebhookUrl:  strp("https://example.com/pre"),
			PostWebhookUrl: strp("https://example.com/post"),
			Target:         strp("webhook"),
		},
	}
}

func TestMigrateConversations_FullSuccess(t *testing.T) {
	dst := &fakeConvDest{}
	res, err := migrateConversations(context.Background(), fullConvSource(), dst, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(dst.createdServices) != 1 || dst.createdServices[0].FriendlyName != "Support" {
		t.Errorf("service not created: %+v", dst.createdServices)
	}
	if len(dst.createdRoles) != 1 || dst.createdRoles[0].Type != "conversation" || len(dst.createdRoles[0].Permission) != 1 {
		t.Errorf("role not created: %+v", dst.createdRoles)
	}
	if len(dst.createdUsers) != 1 || dst.createdUsers[0].Identity != "alice" || dst.createdUsers[0].RoleSid == nil || *dst.createdUsers[0].RoleSid != "RLnew" {
		t.Errorf("user not created or role not bridged: %+v", dst.createdUsers)
	}
	if len(dst.createdConversations) != 2 {
		t.Errorf("conversations not created: %+v", dst.createdConversations)
	}
	if len(dst.createdParticipants["CHnewconv"]) != 2 {
		t.Errorf("participants not created: %+v", dst.createdParticipants)
	}
	if dst.createdParticipants["CHnewconv"][0].RoleSid == nil || *dst.createdParticipants["CHnewconv"][0].RoleSid != "RLnew" {
		t.Errorf("participant role not bridged: %+v", dst.createdParticipants["CHnewconv"][0])
	}
	if len(dst.createdMessages["CHnewconv"]) != 1 || dst.createdMessages["CHnewconv"][0].Body == nil || *dst.createdMessages["CHnewconv"][0].Body != "hi" {
		t.Errorf("message not created: %+v", dst.createdMessages)
	}
	if len(dst.createdWebhooks["CHnewconv"]) != 1 {
		t.Errorf("webhook not created: %+v", dst.createdWebhooks)
	}
	wh := dst.createdWebhooks["CHnewconv"][0]
	if wh.ConfigurationURL == nil || *wh.ConfigurationURL != "https://example.com/hook" || wh.ConfigurationMethod == nil || *wh.ConfigurationMethod != "POST" {
		t.Errorf("webhook config not extracted: %+v", wh)
	}
	if len(dst.createdConfigAddrs) != 1 || dst.createdConfigAddrs[0].Address != "+15559876543" {
		t.Errorf("config address not created: %+v", dst.createdConfigAddrs)
	}
	if dst.updatedConfig == nil || dst.updatedConfig.DefaultInactiveTimer == nil || *dst.updatedConfig.DefaultInactiveTimer != "PT1H" {
		t.Errorf("configuration not synced: %+v", dst.updatedConfig)
	}
	if dst.updatedConfigWebhook == nil || dst.updatedConfigWebhook.Method == nil || *dst.updatedConfigWebhook.Method != "POST" {
		t.Errorf("configuration webhook not synced: %+v", dst.updatedConfigWebhook)
	}
	if res.HasFailures() {
		t.Errorf("expected no failures, got %+v", res.Items)
	}
}

func TestMigrateConversations_DryRun(t *testing.T) {
	dst := &fakeConvDest{}
	res, err := migrateConversations(context.Background(), fullConvSource(), dst, Options{DryRun: true})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(dst.createdServices)+len(dst.createdRoles)+len(dst.createdUsers)+len(dst.createdConversations)+len(dst.createdConfigAddrs) != 0 {
		t.Errorf("dry run must not create: %+v", dst)
	}
	if dst.updatedConfig != nil || dst.updatedConfigWebhook != nil {
		t.Errorf("dry run must not update configuration: %+v / %+v", dst.updatedConfig, dst.updatedConfigWebhook)
	}
	if res.Count(StatusPlanned) == 0 {
		t.Errorf("expected planned items, got %+v", res.Items)
	}
}

// --- empty-key / empty-name failures ---

func TestMigrateConversations_EmptyNames(t *testing.T) {
	src := fakeConvSource{
		services:      []twconv.ConversationsV1Service{{Sid: strp("IS1")}},
		roles:         []twconv.ConversationsV1Role{{Sid: strp("RL1")}},
		users:         []twconv.ConversationsV1User{{Sid: strp("US1")}},
		conversations: []twconv.ConversationsV1Conversation{{Sid: strp("CH1")}},
		configAddrs:   []twconv.ConversationsV1ConfigurationAddress{{Sid: strp("IG1")}},
	}
	res, err := migrateConversations(context.Background(), src, &fakeConvDest{}, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusFailed) != 5 {
		t.Errorf("expected 5 failed items (service/role/user/conversation/config-address), got %d: %+v", res.Count(StatusFailed), res.Items)
	}
}

func TestMigrateConversations_EmptyParticipantKey(t *testing.T) {
	src := fakeConvSource{
		conversations: []twconv.ConversationsV1Conversation{{Sid: strp("CH1"), UniqueName: strp("thread")}},
		participants:  map[string][]twconv.ConversationsV1ConversationParticipant{"CH1": {{}}},
	}
	res, err := migrateConversations(context.Background(), src, &fakeConvDest{}, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed participant, got %+v", res.Items)
	}
}

func TestMigrateConversations_EmptyWebhookTarget(t *testing.T) {
	src := fakeConvSource{
		conversations: []twconv.ConversationsV1Conversation{{Sid: strp("CH1"), UniqueName: strp("thread")}},
		webhooks:      map[string][]twconv.ConversationsV1ConversationScopedWebhook{"CH1": {{}}},
	}
	res, err := migrateConversations(context.Background(), src, &fakeConvDest{}, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed webhook, got %+v", res.Items)
	}
}

// --- skips (already present) ---

func TestMigrateConversations_SkipsExisting(t *testing.T) {
	src := fullConvSource()
	dst := &fakeConvDest{
		existingServices:      []voiceml.ConversationsV1ChatService{{FriendlyName: strp("Support")}},
		existingRoles:         []voiceml.ConversationsV1Role{{Sid: strp("RLexist"), FriendlyName: strp("admin")}},
		existingUsers:         []voiceml.ConversationsV1User{{Identity: strp("alice")}},
		existingConversations: []voiceml.ConversationsV1Conversation{{Sid: strp("CHexist"), UniqueName: strp("support-thread")}},
		existingConfigAddrs:   []voiceml.ConversationsV1ConfigAddress{{Type: strp("sms"), Address: strp("+15559876543")}},
	}
	dst.existingParticipants = map[string][]voiceml.ConversationsV1ConversationParticipant{
		"CHexist": {{Identity: strp("alice")}, {MessagingBinding: map[string]string{"address": "+15551234567"}}},
	}
	dst.existingMessages = map[string][]voiceml.ConversationsV1ConversationMessage{"CHexist": {{Sid: strp("MSexist")}}}
	dst.existingWebhooks = map[string][]voiceml.ConversationsV1ConversationScopedWebhook{"CHexist": {{Target: strp("webhook")}}}

	res, err := migrateConversations(context.Background(), src, dst, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(dst.createdServices)+len(dst.createdRoles)+len(dst.createdUsers)+len(dst.createdConfigAddrs) != 0 {
		t.Errorf("expected no creates for already-present top-level resources: %+v", dst)
	}
	// The second conversation ("No unique name") still gets created since it's a different key.
	if len(dst.createdConversations) != 1 {
		t.Errorf("expected exactly 1 new conversation, got %+v", dst.createdConversations)
	}
	if len(dst.createdParticipants["CHexist"]) != 0 {
		t.Errorf("expected no new participants against the matched existing conversation: %+v", dst.createdParticipants)
	}
	if len(dst.createdMessages["CHexist"]) != 0 {
		t.Errorf("expected messages skipped (existing count >= source count): %+v", dst.createdMessages)
	}
	if len(dst.createdWebhooks["CHexist"]) != 0 {
		t.Errorf("expected webhook skipped: %+v", dst.createdWebhooks)
	}
	if res.Count(StatusSkipped) == 0 {
		t.Errorf("expected skipped items, got %+v", res.Items)
	}
}

// TestMigrateConversations_ExistingConversationKeyedByFriendlyName covers
// the branch where an EXISTING VoiceML conversation has no UniqueName and
// its key falls back to FriendlyName (the source's second conversation
// matches it and is skipped instead of created).
func TestMigrateConversations_ExistingConversationKeyedByFriendlyName(t *testing.T) {
	src := fakeConvSource{
		conversations: []twconv.ConversationsV1Conversation{{Sid: strp("CH1"), FriendlyName: strp("No unique name")}},
	}
	dst := &fakeConvDest{
		existingConversations: []voiceml.ConversationsV1Conversation{{Sid: strp("CHexist"), FriendlyName: strp("No unique name")}},
	}
	res, err := migrateConversations(context.Background(), src, dst, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(dst.createdConversations) != 0 {
		t.Errorf("expected the conversation to match by friendly name and be skipped, got %+v", dst.createdConversations)
	}
	if res.Items[0].Status != StatusSkipped {
		t.Errorf("expected the conversation item to be skipped, got %+v", res.Items[0])
	}
}

// TestMigrateConversations_DryRunCascadeAgainstExisting covers the
// DryRun-planned branches inside migrateConvParticipants and
// migrateConvWebhooks specifically: those only fire when the conversation
// itself already exists (so newSID is populated even under DryRun) and the
// nested resource is new. A brand-new conversation under DryRun never gets a
// SID, so its cascade never runs at all (see TestMigrateConversations_DryRun).
func TestMigrateConversations_DryRunCascadeAgainstExisting(t *testing.T) {
	src := fakeConvSource{
		conversations: []twconv.ConversationsV1Conversation{{Sid: strp("CH1"), UniqueName: strp("thread")}},
		participants:  map[string][]twconv.ConversationsV1ConversationParticipant{"CH1": {{Identity: strp("bob")}}},
		webhooks:      map[string][]twconv.ConversationsV1ConversationScopedWebhook{"CH1": {{Target: strp("webhook")}}},
	}
	dst := &fakeConvDest{
		existingConversations: []voiceml.ConversationsV1Conversation{{Sid: strp("CHexist"), UniqueName: strp("thread")}},
	}
	res, err := migrateConversations(context.Background(), src, dst, Options{DryRun: true})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusPlanned) < 2 { // participant + webhook
		t.Errorf("expected planned participant and webhook, got %+v", res.Items)
	}
}

// --- create errors ---

func TestMigrateConversations_CreateErrors(t *testing.T) {
	src := fullConvSource()
	dst := &fakeConvDest{
		createServiceErr:       errors.New("x"),
		createRoleErr:          errors.New("x"),
		createUserErr:          errors.New("x"),
		createConvErr:          errors.New("x"),
		createConfigAddrErr:    errors.New("x"),
		updateConfigErr:        errors.New("x"),
		updateConfigWebhookErr: errors.New("x"),
	}
	res, err := migrateConversations(context.Background(), src, dst, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// service, role, user, conversation x2 (both fail to create so cascades never run), config-address, configuration, configuration-webhook
	if res.Count(StatusFailed) < 7 {
		t.Errorf("expected at least 7 failed items, got %d: %+v", res.Count(StatusFailed), res.Items)
	}
}

func TestMigrateConversations_NestedCreateErrors(t *testing.T) {
	src := fullConvSource()
	dst := &fakeConvDest{
		createParticipantErr: errors.New("x"),
		createMessageErr:     errors.New("x"),
		createWebhookErr:     errors.New("x"),
	}
	res, err := migrateConversations(context.Background(), src, dst, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusFailed) < 4 { // 2 participants + 1 message + 1 webhook
		t.Errorf("expected nested creation failures, got %d: %+v", res.Count(StatusFailed), res.Items)
	}
}

// --- messages: dry-run planned branch specifically (already covered by DryRun test,
// but isolate the "would create N" detail branch) ---

func TestMigrateConvMessages_DryRunDetail(t *testing.T) {
	src := fakeConvSource{messages: map[string][]twconv.ConversationsV1ConversationMessage{"CH1": {{Body: strp("hi")}}}}
	res := &Result{}
	if err := migrateConvMessages(context.Background(), src, &fakeConvDest{}, Options{DryRun: true}, "CH1", "thread", "CHnew", res); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusPlanned) != 1 {
		t.Errorf("expected 1 planned item, got %+v", res.Items)
	}
}

// --- list error sweep ---

func TestMigrateConversations_ListErrors(t *testing.T) {
	baseSrc := func() fakeConvSource { return fullConvSource() }
	cases := []struct {
		name string
		src  fakeConvSource
		dst  *fakeConvDest
	}{
		{"twilio services", func() fakeConvSource { s := baseSrc(); s.serviceErr = errors.New("x"); return s }(), &fakeConvDest{}},
		{"voiceml services", baseSrc(), &fakeConvDest{listServiceErr: errors.New("x")}},
		{"twilio roles", func() fakeConvSource { s := baseSrc(); s.roleErr = errors.New("x"); return s }(), &fakeConvDest{}},
		{"voiceml roles", baseSrc(), &fakeConvDest{listRoleErr: errors.New("x")}},
		{"twilio users", func() fakeConvSource { s := baseSrc(); s.userErr = errors.New("x"); return s }(), &fakeConvDest{}},
		{"voiceml users", baseSrc(), &fakeConvDest{listUserErr: errors.New("x")}},
		{"twilio conversations", func() fakeConvSource { s := baseSrc(); s.conversationErr = errors.New("x"); return s }(), &fakeConvDest{}},
		{"voiceml conversations", baseSrc(), &fakeConvDest{listConvErr: errors.New("x")}},
		{"twilio participants", func() fakeConvSource { s := baseSrc(); s.participantErr = errors.New("x"); return s }(), &fakeConvDest{}},
		{"voiceml participants", baseSrc(), &fakeConvDest{listParticipantErr: errors.New("x")}},
		{"twilio messages", func() fakeConvSource { s := baseSrc(); s.messageErr = errors.New("x"); return s }(), &fakeConvDest{}},
		{"voiceml messages", baseSrc(), &fakeConvDest{listMessageErr: errors.New("x")}},
		{"twilio webhooks", func() fakeConvSource { s := baseSrc(); s.webhookErr = errors.New("x"); return s }(), &fakeConvDest{}},
		{"voiceml webhooks", baseSrc(), &fakeConvDest{listWebhookErr: errors.New("x")}},
		{"twilio config addresses", func() fakeConvSource { s := baseSrc(); s.configAddrErr = errors.New("x"); return s }(), &fakeConvDest{}},
		{"voiceml config addresses", baseSrc(), &fakeConvDest{listConfigAddrErr: errors.New("x")}},
		{"twilio configuration", func() fakeConvSource { s := baseSrc(); s.configErr = errors.New("x"); return s }(), &fakeConvDest{}},
		{"twilio configuration webhook", func() fakeConvSource { s := baseSrc(); s.configWebhookErr = errors.New("x"); return s }(), &fakeConvDest{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := migrateConversations(context.Background(), c.src, c.dst, Options{}); err == nil {
				t.Errorf("%s: want error", c.name)
			}
		})
	}
}

// --- helper functions ---

func TestConvKey(t *testing.T) {
	if got := convKey(twconv.ConversationsV1Conversation{UniqueName: strp("u"), FriendlyName: strp("f")}); got != "u" {
		t.Errorf("convKey prefers UniqueName, got %q", got)
	}
	if got := convKey(twconv.ConversationsV1Conversation{FriendlyName: strp("f")}); got != "f" {
		t.Errorf("convKey falls back to FriendlyName, got %q", got)
	}
	if got := convKey(twconv.ConversationsV1Conversation{}); got != "" {
		t.Errorf("convKey empty = %q", got)
	}
}

func TestParticipantKey(t *testing.T) {
	if got := participantKey(twconv.ConversationsV1ConversationParticipant{Identity: strp("alice")}); got != "alice" {
		t.Errorf("participantKey identity = %q", got)
	}
	binding := interface{}(map[string]interface{}{"address": "+1555"})
	if got := participantKey(twconv.ConversationsV1ConversationParticipant{MessagingBinding: &binding}); got != "+1555" {
		t.Errorf("participantKey binding = %q", got)
	}
	if got := participantKey(twconv.ConversationsV1ConversationParticipant{}); got != "" {
		t.Errorf("participantKey empty = %q", got)
	}
	badBinding := interface{}("not-a-map")
	if got := participantKey(twconv.ConversationsV1ConversationParticipant{MessagingBinding: &badBinding}); got != "" {
		t.Errorf("participantKey malformed binding = %q, want empty", got)
	}
	emptyMapBinding := interface{}(map[string]interface{}{})
	if got := participantKey(twconv.ConversationsV1ConversationParticipant{MessagingBinding: &emptyMapBinding}); got != "" {
		t.Errorf("participantKey binding without address = %q, want empty", got)
	}
}

func TestWebhookConfigFields(t *testing.T) {
	url, method, flow := webhookConfigFields(nil)
	if url != nil || method != nil || flow != nil {
		t.Errorf("nil config should return all-nil, got %v %v %v", url, method, flow)
	}

	bad := interface{}("not-a-map")
	url, method, flow = webhookConfigFields(&bad)
	if url != nil || method != nil || flow != nil {
		t.Errorf("malformed config should return all-nil, got %v %v %v", url, method, flow)
	}

	studio := interface{}(map[string]interface{}{"flow_sid": "FWxxx"})
	url, method, flow = webhookConfigFields(&studio)
	if url != nil || method != nil || flow == nil || *flow != "FWxxx" {
		t.Errorf("studio config extraction wrong: url=%v method=%v flow=%v", url, method, flow)
	}
}

func TestConversationsName(t *testing.T) {
	if (Conversations{}).Name() != "conversations" {
		t.Errorf("name=%q", (Conversations{}).Name())
	}
}
