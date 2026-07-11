package migrate

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"testing"

	twapi "github.com/twilio/twilio-go/rest/api/v2010"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

// newRun builds a *sipRun with empty maps, ready for a test to call one
// method directly (surgical branch coverage, bypassing migrateSIP's gating).
func newRun(src sipSource, domains sipDomainsDest, credLists sipCredListsDest, acls sipACLsDest, opts Options) *sipRun {
	res := Result{Resource: "sip-trunking"}
	return &sipRun{
		ctx: context.Background(), src: src, domains: domains, credLists: credLists, acls: acls, opts: opts, res: &res,
		credListByName: map[string]string{}, aclByName: map[string]string{}, domainBySID: map[string]string{},
	}
}

// --- fakes ---

type fakeSIPSource struct {
	domains   []twapi.ApiV2010SipDomain
	credLists []twapi.ApiV2010SipCredentialList
	creds     map[string][]twapi.ApiV2010SipCredential // keyed by cred-list sid
	acls      []twapi.ApiV2010SipIpAccessControlList
	ips       map[string][]twapi.ApiV2010SipIpAddress // keyed by acl sid
	credMaps  map[string][]twapi.ApiV2010SipCredentialListMapping
	aclMaps   map[string][]twapi.ApiV2010SipIpAccessControlListMapping

	// Error injection, one per method, for exercising error-propagation
	// branches without a real network dependency.
	domainErr, credListErr, credErr, aclErr, ipErr, credMapErr, aclMapErr error
}

func (f fakeSIPSource) ListSipDomain(*twapi.ListSipDomainParams) ([]twapi.ApiV2010SipDomain, error) {
	if f.domainErr != nil {
		return nil, f.domainErr
	}
	return f.domains, nil
}
func (f fakeSIPSource) ListSipCredentialList(*twapi.ListSipCredentialListParams) ([]twapi.ApiV2010SipCredentialList, error) {
	if f.credListErr != nil {
		return nil, f.credListErr
	}
	return f.credLists, nil
}
func (f fakeSIPSource) ListSipCredential(listSid string, _ *twapi.ListSipCredentialParams) ([]twapi.ApiV2010SipCredential, error) {
	if f.credErr != nil {
		return nil, f.credErr
	}
	return f.creds[listSid], nil
}
func (f fakeSIPSource) ListSipIpAccessControlList(*twapi.ListSipIpAccessControlListParams) ([]twapi.ApiV2010SipIpAccessControlList, error) {
	if f.aclErr != nil {
		return nil, f.aclErr
	}
	return f.acls, nil
}
func (f fakeSIPSource) ListSipIpAddress(aclSid string, _ *twapi.ListSipIpAddressParams) ([]twapi.ApiV2010SipIpAddress, error) {
	if f.ipErr != nil {
		return nil, f.ipErr
	}
	return f.ips[aclSid], nil
}
func (f fakeSIPSource) ListSipCredentialListMapping(domainSid string, _ *twapi.ListSipCredentialListMappingParams) ([]twapi.ApiV2010SipCredentialListMapping, error) {
	if f.credMapErr != nil {
		return nil, f.credMapErr
	}
	return f.credMaps[domainSid], nil
}
func (f fakeSIPSource) ListSipIpAccessControlListMapping(domainSid string, _ *twapi.ListSipIpAccessControlListMappingParams) ([]twapi.ApiV2010SipIpAccessControlListMapping, error) {
	if f.aclMapErr != nil {
		return nil, f.aclMapErr
	}
	return f.aclMaps[domainSid], nil
}

type fakeSIPCredLists struct {
	existing     []voiceml.SIPCredentialList
	existingCred []voiceml.SIPCredential
	created      []string // friendly names
	credentials  []voiceml.CreateSIPCredentialParams
	nextSID      int

	listErr, createErr, listCredErr, createCredErr error
}

func (f *fakeSIPCredLists) List(context.Context, voiceml.ListPageParams) (*voiceml.SIPCredentialListList, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return &voiceml.SIPCredentialListList{CredentialLists: f.existing}, nil
}
func (f *fakeSIPCredLists) Create(_ context.Context, p voiceml.CreateSIPCredentialListParams) (*voiceml.SIPCredentialList, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = append(f.created, p.FriendlyName)
	f.nextSID++
	return &voiceml.SIPCredentialList{Sid: "CLnew", FriendlyName: strp(p.FriendlyName)}, nil
}
func (f *fakeSIPCredLists) ListCredentials(context.Context, string, voiceml.ListPageParams) (*voiceml.SIPCredentialPage, error) {
	if f.listCredErr != nil {
		return nil, f.listCredErr
	}
	return &voiceml.SIPCredentialPage{Credentials: f.existingCred}, nil
}
func (f *fakeSIPCredLists) CreateCredential(_ context.Context, _ string, p voiceml.CreateSIPCredentialParams) (*voiceml.SIPCredential, error) {
	if f.createCredErr != nil {
		return nil, f.createCredErr
	}
	f.credentials = append(f.credentials, p)
	return &voiceml.SIPCredential{Username: p.Username}, nil
}

type fakeSIPACLs struct {
	existing   []voiceml.SIPIpAccessControlList
	existingIP []voiceml.SIPIpAddress
	created    []string
	ips        []voiceml.CreateSIPIpAddressParams

	listErr, createErr, listIPErr, createIPErr error
}

func (f *fakeSIPACLs) List(context.Context, voiceml.ListPageParams) (*voiceml.SIPIpAccessControlListList, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return &voiceml.SIPIpAccessControlListList{IpAccessControlLists: f.existing}, nil
}
func (f *fakeSIPACLs) Create(_ context.Context, p voiceml.CreateSIPIpAccessControlListParams) (*voiceml.SIPIpAccessControlList, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = append(f.created, p.FriendlyName)
	return &voiceml.SIPIpAccessControlList{Sid: "ALnew", FriendlyName: strp(p.FriendlyName)}, nil
}
func (f *fakeSIPACLs) ListIpAddresses(context.Context, string, voiceml.ListPageParams) (*voiceml.SIPIpAddressList, error) {
	if f.listIPErr != nil {
		return nil, f.listIPErr
	}
	return &voiceml.SIPIpAddressList{IpAddresses: f.existingIP}, nil
}
func (f *fakeSIPACLs) CreateIpAddress(_ context.Context, _ string, p voiceml.CreateSIPIpAddressParams) (*voiceml.SIPIpAddress, error) {
	if f.createIPErr != nil {
		return nil, f.createIPErr
	}
	f.ips = append(f.ips, p)
	return &voiceml.SIPIpAddress{IpAddress: p.IpAddress}, nil
}

type fakeSIPDomains struct {
	existing        []voiceml.SIPDomain
	existingCredMap []voiceml.SIPDomainMapping
	existingACLMap  []voiceml.SIPDomainMapping
	created         []string
	credMaps        []string // credential-list SIDs mapped
	aclMaps         []string // ACL SIDs mapped

	listErr, createErr, listCredMapErr, createCredMapErr, listACLMapErr, createACLMapErr error
}

func (f *fakeSIPDomains) List(context.Context, voiceml.ListPageParams) (*voiceml.SIPDomainList, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return &voiceml.SIPDomainList{Domains: f.existing}, nil
}
func (f *fakeSIPDomains) Create(_ context.Context, p voiceml.CreateSIPDomainParams) (*voiceml.SIPDomain, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = append(f.created, p.DomainName)
	return &voiceml.SIPDomain{Sid: "SDnew", DomainName: p.DomainName}, nil
}
func (f *fakeSIPDomains) ListCredentialListMappings(context.Context, string, voiceml.ListPageParams) (*voiceml.SIPCredentialListMappingList, error) {
	if f.listCredMapErr != nil {
		return nil, f.listCredMapErr
	}
	return &voiceml.SIPCredentialListMappingList{CredentialListMappings: f.existingCredMap}, nil
}
func (f *fakeSIPDomains) CreateCredentialListMapping(_ context.Context, _ string, p voiceml.CreateSIPCredentialListMappingParams) (*voiceml.SIPDomainMapping, error) {
	if f.createCredMapErr != nil {
		return nil, f.createCredMapErr
	}
	f.credMaps = append(f.credMaps, p.CredentialListSid)
	return &voiceml.SIPDomainMapping{}, nil
}
func (f *fakeSIPDomains) ListIpAccessControlListMappings(context.Context, string, voiceml.ListPageParams) (*voiceml.SIPIpAccessControlListMappingList, error) {
	if f.listACLMapErr != nil {
		return nil, f.listACLMapErr
	}
	return &voiceml.SIPIpAccessControlListMappingList{IpAccessControlListMappings: f.existingACLMap}, nil
}
func (f *fakeSIPDomains) CreateIpAccessControlListMapping(_ context.Context, _ string, p voiceml.CreateSIPIpAccessControlListMappingParams) (*voiceml.SIPDomainMapping, error) {
	if f.createACLMapErr != nil {
		return nil, f.createACLMapErr
	}
	f.aclMaps = append(f.aclMaps, p.IpAccessControlListSid)
	return &voiceml.SIPDomainMapping{}, nil
}

// --- tests ---

func sampleSIPSource() fakeSIPSource {
	return fakeSIPSource{
		credLists: []twapi.ApiV2010SipCredentialList{{Sid: strp("CLold"), FriendlyName: strp("Phones")}},
		creds:     map[string][]twapi.ApiV2010SipCredential{"CLold": {{Username: strp("7501")}}},
		acls:      []twapi.ApiV2010SipIpAccessControlList{{Sid: strp("ALold"), FriendlyName: strp("Office")}},
		ips:       map[string][]twapi.ApiV2010SipIpAddress{"ALold": {{FriendlyName: strp("hq"), IpAddress: strp("203.0.113.4"), CidrPrefixLength: 32}}},
		domains:   []twapi.ApiV2010SipDomain{{Sid: strp("SDold"), DomainName: strp("acme.vml.voice.tel")}},
		credMaps:  map[string][]twapi.ApiV2010SipCredentialListMapping{"SDold": {{FriendlyName: strp("Phones")}}},
		aclMaps:   map[string][]twapi.ApiV2010SipIpAccessControlListMapping{"SDold": {{FriendlyName: strp("Office")}}},
	}
}

func TestMigrateSIP_FullGraph(t *testing.T) {
	src := sampleSIPSource()
	domains, credLists, acls := &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}

	res, err := migrateSIP(context.Background(), src, domains, credLists, acls, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if len(credLists.created) != 1 || credLists.created[0] != "Phones" {
		t.Errorf("cred list not created: %+v", credLists.created)
	}
	if len(credLists.credentials) != 1 || credLists.credentials[0].Username != "7501" {
		t.Errorf("credential not created: %+v", credLists.credentials)
	}
	if len(credLists.credentials[0].Password) != generatedPasswordLen {
		t.Errorf("credential password not generated: %q", credLists.credentials[0].Password)
	}
	if len(acls.created) != 1 || len(acls.ips) != 1 || acls.ips[0].IpAddress != "203.0.113.4" {
		t.Errorf("ACL/IP not created: %+v / %+v", acls.created, acls.ips)
	}
	if len(domains.created) != 1 || domains.created[0] != "acme.vml.voice.tel" {
		t.Errorf("domain not created: %+v", domains.created)
	}
	// Mappings must be re-pointed at the NEW VoiceML SIDs, not the Twilio ones.
	if len(domains.credMaps) != 1 || domains.credMaps[0] != "CLnew" {
		t.Errorf("cred-list mapping not remapped: %+v", domains.credMaps)
	}
	if len(domains.aclMaps) != 1 || domains.aclMaps[0] != "ALnew" {
		t.Errorf("ACL mapping not remapped: %+v", domains.aclMaps)
	}

	// The generated password must be surfaced in a created-credential item.
	var found bool
	for _, it := range res.Items {
		if strings.HasPrefix(it.ID, "credential 7501") && it.Status == StatusCreated && strings.Contains(it.Detail, "NEW generated password") {
			found = true
		}
	}
	if !found {
		t.Errorf("generated password not reported: %+v", res.Items)
	}
}

func TestMigrateSIP_DryRun(t *testing.T) {
	src := sampleSIPSource()
	domains, credLists, acls := &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}

	res, err := migrateSIP(context.Background(), src, domains, credLists, acls, Options{DryRun: true})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(credLists.created) != 0 || len(domains.created) != 0 || len(acls.created) != 0 {
		t.Errorf("dry run must not write")
	}
	if res.Count(StatusPlanned) == 0 {
		t.Errorf("expected planned items in dry run: %+v", res)
	}
	// Mappings are skipped in dry-run because the domain has no new SID yet.
}

func TestMigrateSIP_SkipsExisting(t *testing.T) {
	src := sampleSIPSource()
	// Everything already exists on VoiceML → every item should be skipped and
	// nothing created.
	domains := &fakeSIPDomains{
		existing:        []voiceml.SIPDomain{{Sid: "SDx", DomainName: "acme.vml.voice.tel"}},
		existingCredMap: []voiceml.SIPDomainMapping{{Sid: "CLx"}},
		existingACLMap:  []voiceml.SIPDomainMapping{{Sid: "ALx"}},
	}
	credLists := &fakeSIPCredLists{
		existing:     []voiceml.SIPCredentialList{{Sid: "CLx", FriendlyName: strp("Phones")}},
		existingCred: []voiceml.SIPCredential{{Username: "7501"}},
	}
	acls := &fakeSIPACLs{
		existing:   []voiceml.SIPIpAccessControlList{{Sid: "ALx", FriendlyName: strp("Office")}},
		existingIP: []voiceml.SIPIpAddress{{IpAddress: "203.0.113.4"}},
	}

	res, err := migrateSIP(context.Background(), src, domains, credLists, acls, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(credLists.created)+len(credLists.credentials)+len(acls.created)+len(acls.ips)+len(domains.created)+len(domains.credMaps)+len(domains.aclMaps) != 0 {
		t.Errorf("nothing should be created when all resources exist")
	}
	if res.Count(StatusCreated) != 0 || res.Count(StatusSkipped) == 0 {
		t.Errorf("expected all-skipped, got %+v", res)
	}
}

func TestSIPName(t *testing.T) {
	if (SIP{}).Name() != "sip-trunking" {
		t.Errorf("name=%q", (SIP{}).Name())
	}
}

// --- migrateCredentialLists branch coverage ---

func TestMigrateCredentialLists_TwilioListError(t *testing.T) {
	r := newRun(fakeSIPSource{credListErr: errors.New("boom")}, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err := r.migrateCredentialLists(); err == nil || !strings.Contains(err.Error(), "list Twilio SIP credential lists") {
		t.Fatalf("got %v", err)
	}
}

func TestMigrateCredentialLists_VoiceMLListError(t *testing.T) {
	r := newRun(fakeSIPSource{}, &fakeSIPDomains{}, &fakeSIPCredLists{listErr: errors.New("boom")}, &fakeSIPACLs{}, Options{})
	if err := r.migrateCredentialLists(); err == nil || !strings.Contains(err.Error(), "list VoiceML SIP credential lists") {
		t.Fatalf("got %v", err)
	}
}

func TestMigrateCredentialLists_EmptyName(t *testing.T) {
	src := fakeSIPSource{credLists: []twapi.ApiV2010SipCredentialList{{Sid: strp("CL1")}}}
	r := newRun(src, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err := r.migrateCredentialLists(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if r.res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", r.res.Items)
	}
}

func TestMigrateCredentialLists_CreateError(t *testing.T) {
	src := fakeSIPSource{credLists: []twapi.ApiV2010SipCredentialList{{Sid: strp("CL1"), FriendlyName: strp("Phones")}}}
	r := newRun(src, &fakeSIPDomains{}, &fakeSIPCredLists{createErr: errors.New("boom")}, &fakeSIPACLs{}, Options{})
	if err := r.migrateCredentialLists(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if r.res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", r.res.Items)
	}
}

// --- migrateCredentials branch coverage ---

func TestMigrateCredentials_TwilioListError(t *testing.T) {
	r := newRun(fakeSIPSource{credErr: errors.New("boom")}, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err := r.migrateCredentials("CL1", "Phones", "CLnew"); err == nil || !strings.Contains(err.Error(), "list Twilio SIP credentials for Phones") {
		t.Fatalf("got %v", err)
	}
}

func TestMigrateCredentials_VoiceMLListError(t *testing.T) {
	r := newRun(fakeSIPSource{}, &fakeSIPDomains{}, &fakeSIPCredLists{listCredErr: errors.New("boom")}, &fakeSIPACLs{}, Options{})
	if err := r.migrateCredentials("CL1", "Phones", "CLnew"); err == nil || !strings.Contains(err.Error(), "list VoiceML SIP credentials") {
		t.Fatalf("got %v", err)
	}
}

func TestMigrateCredentials_EmptyUsername(t *testing.T) {
	src := fakeSIPSource{creds: map[string][]twapi.ApiV2010SipCredential{"CL1": {{}}}}
	r := newRun(src, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err := r.migrateCredentials("CL1", "Phones", "CLnew"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if r.res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", r.res.Items)
	}
}

func TestMigrateCredentials_GeneratePasswordError(t *testing.T) {
	orig := rand.Reader
	rand.Reader = failingReader{}
	defer func() { rand.Reader = orig }()

	src := fakeSIPSource{creds: map[string][]twapi.ApiV2010SipCredential{"CL1": {{Username: strp("7501")}}}}
	r := newRun(src, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err := r.migrateCredentials("CL1", "Phones", "CLnew"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if r.res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", r.res.Items)
	}
}

func TestMigrateCredentials_CreateError(t *testing.T) {
	src := fakeSIPSource{creds: map[string][]twapi.ApiV2010SipCredential{"CL1": {{Username: strp("7501")}}}}
	r := newRun(src, &fakeSIPDomains{}, &fakeSIPCredLists{createCredErr: errors.New("boom")}, &fakeSIPACLs{}, Options{})
	if err := r.migrateCredentials("CL1", "Phones", "CLnew"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if r.res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", r.res.Items)
	}
}

// --- migrateAccessControlLists branch coverage ---

func TestMigrateAccessControlLists_TwilioListError(t *testing.T) {
	r := newRun(fakeSIPSource{aclErr: errors.New("boom")}, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err := r.migrateAccessControlLists(); err == nil || !strings.Contains(err.Error(), "list Twilio SIP IP ACLs") {
		t.Fatalf("got %v", err)
	}
}

func TestMigrateAccessControlLists_VoiceMLListError(t *testing.T) {
	r := newRun(fakeSIPSource{}, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{listErr: errors.New("boom")}, Options{})
	if err := r.migrateAccessControlLists(); err == nil || !strings.Contains(err.Error(), "list VoiceML SIP IP ACLs") {
		t.Fatalf("got %v", err)
	}
}

func TestMigrateAccessControlLists_EmptyName(t *testing.T) {
	src := fakeSIPSource{acls: []twapi.ApiV2010SipIpAccessControlList{{Sid: strp("AL1")}}}
	r := newRun(src, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err := r.migrateAccessControlLists(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if r.res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", r.res.Items)
	}
}

func TestMigrateAccessControlLists_CreateError(t *testing.T) {
	src := fakeSIPSource{acls: []twapi.ApiV2010SipIpAccessControlList{{Sid: strp("AL1"), FriendlyName: strp("Office")}}}
	r := newRun(src, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{createErr: errors.New("boom")}, Options{})
	if err := r.migrateAccessControlLists(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if r.res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", r.res.Items)
	}
}

// --- migrateIPAddresses branch coverage ---

func TestMigrateIPAddresses_TwilioListError(t *testing.T) {
	r := newRun(fakeSIPSource{ipErr: errors.New("boom")}, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err := r.migrateIPAddresses("AL1", "Office", "ALnew"); err == nil || !strings.Contains(err.Error(), "list Twilio SIP IP addresses for Office") {
		t.Fatalf("got %v", err)
	}
}

func TestMigrateIPAddresses_VoiceMLListError(t *testing.T) {
	r := newRun(fakeSIPSource{}, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{listIPErr: errors.New("boom")}, Options{})
	if err := r.migrateIPAddresses("AL1", "Office", "ALnew"); err == nil || !strings.Contains(err.Error(), "list VoiceML SIP IP addresses") {
		t.Fatalf("got %v", err)
	}
}

func TestMigrateIPAddresses_EmptyAddr(t *testing.T) {
	src := fakeSIPSource{ips: map[string][]twapi.ApiV2010SipIpAddress{"AL1": {{}}}}
	r := newRun(src, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err := r.migrateIPAddresses("AL1", "Office", "ALnew"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if r.res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", r.res.Items)
	}
}

func TestMigrateIPAddresses_CreateError(t *testing.T) {
	src := fakeSIPSource{ips: map[string][]twapi.ApiV2010SipIpAddress{"AL1": {{IpAddress: strp("203.0.113.4")}}}}
	r := newRun(src, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{createIPErr: errors.New("boom")}, Options{})
	if err := r.migrateIPAddresses("AL1", "Office", "ALnew"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if r.res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", r.res.Items)
	}
}

func TestMigrateIPAddresses_NoCidr(t *testing.T) {
	// CidrPrefixLength <= 0 must not set params.CidrPrefixLength.
	src := fakeSIPSource{ips: map[string][]twapi.ApiV2010SipIpAddress{"AL1": {{IpAddress: strp("203.0.113.5"), CidrPrefixLength: 0}}}}
	acls := &fakeSIPACLs{}
	r := newRun(src, &fakeSIPDomains{}, &fakeSIPCredLists{}, acls, Options{})
	if err := r.migrateIPAddresses("AL1", "Office", "ALnew"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(acls.ips) != 1 || acls.ips[0].CidrPrefixLength != nil {
		t.Errorf("expected no CIDR set, got %+v", acls.ips)
	}
}

// --- migrateDomains branch coverage ---

func TestMigrateDomains_TwilioListError(t *testing.T) {
	r := newRun(fakeSIPSource{domainErr: errors.New("boom")}, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err := r.migrateDomains(); err == nil || !strings.Contains(err.Error(), "list Twilio SIP domains") {
		t.Fatalf("got %v", err)
	}
}

func TestMigrateDomains_VoiceMLListError(t *testing.T) {
	r := newRun(fakeSIPSource{}, &fakeSIPDomains{listErr: errors.New("boom")}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err := r.migrateDomains(); err == nil || !strings.Contains(err.Error(), "list VoiceML SIP domains") {
		t.Fatalf("got %v", err)
	}
}

func TestMigrateDomains_EmptyName(t *testing.T) {
	src := fakeSIPSource{domains: []twapi.ApiV2010SipDomain{{Sid: strp("SD1")}}}
	r := newRun(src, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err := r.migrateDomains(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if r.res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", r.res.Items)
	}
}

func TestMigrateDomains_CreateError(t *testing.T) {
	src := fakeSIPSource{domains: []twapi.ApiV2010SipDomain{{Sid: strp("SD1"), DomainName: strp("acme.vml.voice.tel")}}}
	r := newRun(src, &fakeSIPDomains{createErr: errors.New("boom")}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err := r.migrateDomains(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if r.res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", r.res.Items)
	}
}

// --- migrateMappings (outer) branch coverage ---

func TestMigrateMappings_TwilioListError(t *testing.T) {
	r := newRun(fakeSIPSource{domainErr: errors.New("boom")}, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err := r.migrateMappings(); err == nil || !strings.Contains(err.Error(), "list Twilio SIP domains (mappings)") {
		t.Fatalf("got %v", err)
	}
}

func TestMigrateMappings_SkipsDomainWithoutNewSID(t *testing.T) {
	// A domain with no entry in domainBySID (never created/matched) must be
	// skipped without attempting any mapping calls.
	src := fakeSIPSource{domains: []twapi.ApiV2010SipDomain{{Sid: strp("SD1"), DomainName: strp("acme.vml.voice.tel")}}}
	domains := &fakeSIPDomains{listCredMapErr: errors.New("must not be called")}
	r := newRun(src, domains, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err := r.migrateMappings(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

// --- migrateCredListMappings branch coverage ---

func TestMigrateCredListMappings_TwilioListError(t *testing.T) {
	r := newRun(fakeSIPSource{credMapErr: errors.New("boom")}, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err := r.migrateCredListMappings("SD1", "acme.vml.voice.tel", "SDnew"); err == nil || !strings.Contains(err.Error(), "list Twilio credential-list mappings") {
		t.Fatalf("got %v", err)
	}
}

func TestMigrateCredListMappings_VoiceMLListError(t *testing.T) {
	r := newRun(fakeSIPSource{}, &fakeSIPDomains{listCredMapErr: errors.New("boom")}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err := r.migrateCredListMappings("SD1", "acme.vml.voice.tel", "SDnew"); err == nil || !strings.Contains(err.Error(), "list VoiceML credential-list mappings") {
		t.Fatalf("got %v", err)
	}
}

func TestMigrateCredListMappings_NotMigrated(t *testing.T) {
	src := fakeSIPSource{credMaps: map[string][]twapi.ApiV2010SipCredentialListMapping{"SD1": {{FriendlyName: strp("Phones")}}}}
	r := newRun(src, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err := r.migrateCredListMappings("SD1", "acme.vml.voice.tel", "SDnew"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if r.res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", r.res.Items)
	}
}

func TestMigrateCredListMappings_AlreadyMapped(t *testing.T) {
	src := fakeSIPSource{credMaps: map[string][]twapi.ApiV2010SipCredentialListMapping{"SD1": {{FriendlyName: strp("Phones")}}}}
	domains := &fakeSIPDomains{existingCredMap: []voiceml.SIPDomainMapping{{Sid: "CLnew"}}}
	r := newRun(src, domains, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	r.credListByName["Phones"] = "CLnew"
	if err := r.migrateCredListMappings("SD1", "acme.vml.voice.tel", "SDnew"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if r.res.Count(StatusSkipped) != 1 {
		t.Errorf("expected 1 skipped item, got %+v", r.res.Items)
	}
}

func TestMigrateCredListMappings_DryRunPlanned(t *testing.T) {
	src := fakeSIPSource{credMaps: map[string][]twapi.ApiV2010SipCredentialListMapping{"SD1": {{FriendlyName: strp("Phones")}}}}
	r := newRun(src, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{DryRun: true})
	r.credListByName["Phones"] = "CLnew"
	if err := r.migrateCredListMappings("SD1", "acme.vml.voice.tel", "SDnew"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if r.res.Count(StatusPlanned) != 1 {
		t.Errorf("expected 1 planned item, got %+v", r.res.Items)
	}
}

func TestMigrateCredListMappings_CreateError(t *testing.T) {
	src := fakeSIPSource{credMaps: map[string][]twapi.ApiV2010SipCredentialListMapping{"SD1": {{FriendlyName: strp("Phones")}}}}
	domains := &fakeSIPDomains{createCredMapErr: errors.New("boom")}
	r := newRun(src, domains, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	r.credListByName["Phones"] = "CLnew"
	if err := r.migrateCredListMappings("SD1", "acme.vml.voice.tel", "SDnew"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if r.res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", r.res.Items)
	}
}

// --- migrateACLMappings branch coverage ---

func TestMigrateACLMappings_TwilioListError(t *testing.T) {
	r := newRun(fakeSIPSource{aclMapErr: errors.New("boom")}, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err := r.migrateACLMappings("SD1", "acme.vml.voice.tel", "SDnew"); err == nil || !strings.Contains(err.Error(), "list Twilio IP-ACL mappings") {
		t.Fatalf("got %v", err)
	}
}

func TestMigrateACLMappings_VoiceMLListError(t *testing.T) {
	r := newRun(fakeSIPSource{}, &fakeSIPDomains{listACLMapErr: errors.New("boom")}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err := r.migrateACLMappings("SD1", "acme.vml.voice.tel", "SDnew"); err == nil || !strings.Contains(err.Error(), "list VoiceML IP-ACL mappings") {
		t.Fatalf("got %v", err)
	}
}

func TestMigrateACLMappings_NotMigrated(t *testing.T) {
	src := fakeSIPSource{aclMaps: map[string][]twapi.ApiV2010SipIpAccessControlListMapping{"SD1": {{FriendlyName: strp("Office")}}}}
	r := newRun(src, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err := r.migrateACLMappings("SD1", "acme.vml.voice.tel", "SDnew"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if r.res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", r.res.Items)
	}
}

func TestMigrateACLMappings_AlreadyMapped(t *testing.T) {
	src := fakeSIPSource{aclMaps: map[string][]twapi.ApiV2010SipIpAccessControlListMapping{"SD1": {{FriendlyName: strp("Office")}}}}
	domains := &fakeSIPDomains{existingACLMap: []voiceml.SIPDomainMapping{{Sid: "ALnew"}}}
	r := newRun(src, domains, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	r.aclByName["Office"] = "ALnew"
	if err := r.migrateACLMappings("SD1", "acme.vml.voice.tel", "SDnew"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if r.res.Count(StatusSkipped) != 1 {
		t.Errorf("expected 1 skipped item, got %+v", r.res.Items)
	}
}

func TestMigrateACLMappings_DryRunPlanned(t *testing.T) {
	src := fakeSIPSource{aclMaps: map[string][]twapi.ApiV2010SipIpAccessControlListMapping{"SD1": {{FriendlyName: strp("Office")}}}}
	r := newRun(src, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{DryRun: true})
	r.aclByName["Office"] = "ALnew"
	if err := r.migrateACLMappings("SD1", "acme.vml.voice.tel", "SDnew"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if r.res.Count(StatusPlanned) != 1 {
		t.Errorf("expected 1 planned item, got %+v", r.res.Items)
	}
}

func TestMigrateACLMappings_CreateError(t *testing.T) {
	src := fakeSIPSource{aclMaps: map[string][]twapi.ApiV2010SipIpAccessControlListMapping{"SD1": {{FriendlyName: strp("Office")}}}}
	domains := &fakeSIPDomains{createACLMapErr: errors.New("boom")}
	r := newRun(src, domains, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	r.aclByName["Office"] = "ALnew"
	if err := r.migrateACLMappings("SD1", "acme.vml.voice.tel", "SDnew"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if r.res.Count(StatusFailed) != 1 {
		t.Errorf("expected 1 failed item, got %+v", r.res.Items)
	}
}

// TestMigrateCredentialLists_PropagatesCredentialsError covers the branch in
// migrateCredentialLists where the newly created list's migrateCredentials
// call itself errors and that error propagates up.
func TestMigrateCredentialLists_PropagatesCredentialsError(t *testing.T) {
	src := fakeSIPSource{
		credLists: []twapi.ApiV2010SipCredentialList{{Sid: strp("CL1"), FriendlyName: strp("Phones")}},
		credErr:   errors.New("boom"),
	}
	r := newRun(src, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err := r.migrateCredentialLists(); err == nil || !strings.Contains(err.Error(), "list Twilio SIP credentials for Phones") {
		t.Fatalf("expected migrateCredentials error to propagate, got %v", err)
	}
}

// TestMigrateCredentials_DryRunPlanned covers migrateCredentials' own
// DryRun-planned branch directly (unreachable via the full migrateSIP flow
// since dry-run never creates a credential list to hang credentials off of).
func TestMigrateCredentials_DryRunPlanned(t *testing.T) {
	src := fakeSIPSource{creds: map[string][]twapi.ApiV2010SipCredential{"CL1": {{Username: strp("7501")}}}}
	r := newRun(src, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{DryRun: true})
	if err := r.migrateCredentials("CL1", "Phones", "CLnew"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if r.res.Count(StatusPlanned) != 1 {
		t.Errorf("expected 1 planned item, got %+v", r.res.Items)
	}
}

// TestMigrateAccessControlLists_PropagatesIPAddressesError mirrors
// TestMigrateCredentialLists_PropagatesCredentialsError for the IP-address
// migration branch.
func TestMigrateAccessControlLists_PropagatesIPAddressesError(t *testing.T) {
	src := fakeSIPSource{
		acls:  []twapi.ApiV2010SipIpAccessControlList{{Sid: strp("AL1"), FriendlyName: strp("Office")}},
		ipErr: errors.New("boom"),
	}
	r := newRun(src, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err := r.migrateAccessControlLists(); err == nil || !strings.Contains(err.Error(), "list Twilio SIP IP addresses for Office") {
		t.Fatalf("expected migrateIPAddresses error to propagate, got %v", err)
	}
}

// TestMigrateIPAddresses_DryRunPlanned covers migrateIPAddresses' own
// DryRun-planned branch directly, for the same reason as
// TestMigrateCredentials_DryRunPlanned.
func TestMigrateIPAddresses_DryRunPlanned(t *testing.T) {
	src := fakeSIPSource{ips: map[string][]twapi.ApiV2010SipIpAddress{"AL1": {{IpAddress: strp("203.0.113.4")}}}}
	r := newRun(src, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{DryRun: true})
	if err := r.migrateIPAddresses("AL1", "Office", "ALnew"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if r.res.Count(StatusPlanned) != 1 {
		t.Errorf("expected 1 planned item, got %+v", r.res.Items)
	}
}

// TestMigrateMappings_PropagatesACLMappingsError covers migrateMappings'
// return-err branch after a successful migrateCredListMappings call, which
// TestMigrateMappings_TwilioListError / the credMapErr-based propagation
// test do not reach (those fail before migrateACLMappings ever runs).
func TestMigrateMappings_PropagatesACLMappingsError(t *testing.T) {
	src := fakeSIPSource{
		domains:   []twapi.ApiV2010SipDomain{{Sid: strp("SD1"), DomainName: strp("acme.vml.voice.tel")}},
		aclMaps:   map[string][]twapi.ApiV2010SipIpAccessControlListMapping{"SD1": {{FriendlyName: strp("Office")}}},
		aclMapErr: errors.New("boom"),
	}
	r := newRun(src, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	r.domainBySID["acme.vml.voice.tel"] = "SDnew"
	if err := r.migrateMappings(); err == nil || !strings.Contains(err.Error(), "list Twilio IP-ACL mappings") {
		t.Fatalf("expected migrateACLMappings error to propagate, got %v", err)
	}
}

// --- migrateSIP top-level error propagation ---

func TestMigrateSIP_PropagatesCredentialListsError(t *testing.T) {
	src := fakeSIPSource{credListErr: errors.New("boom")}
	_, err := migrateSIP(context.Background(), src, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err == nil {
		t.Fatal("expected error to propagate from migrateCredentialLists")
	}
}

func TestMigrateSIP_PropagatesAccessControlListsError(t *testing.T) {
	src := fakeSIPSource{aclErr: errors.New("boom")}
	_, err := migrateSIP(context.Background(), src, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err == nil {
		t.Fatal("expected error to propagate from migrateAccessControlLists")
	}
}

func TestMigrateSIP_PropagatesDomainsError(t *testing.T) {
	src := fakeSIPSource{domainErr: errors.New("boom")}
	_, err := migrateSIP(context.Background(), src, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err == nil {
		t.Fatal("expected error to propagate from migrateDomains")
	}
}

func TestMigrateSIP_PropagatesMappingsError(t *testing.T) {
	// migrateDomains succeeds (empty), migrateMappings' own ListSipDomain call
	// fails; since fakeSIPSource is stateless per-call this needs domains
	// non-empty so the first three phases succeed, then mappings' second
	// ListSipDomain call is the one instrumented to fail — but domainErr
	// applies to every call. Use credMapErr/aclMapErr instead to hit the
	// mapping sub-calls via a domain that does get created.
	src := fakeSIPSource{
		domains:    []twapi.ApiV2010SipDomain{{Sid: strp("SD1"), DomainName: strp("acme.vml.voice.tel")}},
		credMapErr: errors.New("boom"),
	}
	_, err := migrateSIP(context.Background(), src, &fakeSIPDomains{}, &fakeSIPCredLists{}, &fakeSIPACLs{}, Options{})
	if err == nil {
		t.Fatal("expected error to propagate from migrateMappings")
	}
}

// TestMigrateSIP_DryRunWithExistingDomainPlansMappings covers the branch
// where a domain already exists on VoiceML (so it has a newDomainSID even in
// dry-run) and its credential-list/ACL mappings do not exist yet — the only
// path that reaches the DryRun-planned branch inside
// migrateCredListMappings/migrateACLMappings via the full migrateSIP flow.
func TestMigrateSIP_DryRunWithExistingDomainPlansMappings(t *testing.T) {
	src := fakeSIPSource{
		domains:   []twapi.ApiV2010SipDomain{{Sid: strp("SD1"), DomainName: strp("acme.vml.voice.tel")}},
		credLists: []twapi.ApiV2010SipCredentialList{{Sid: strp("CL1"), FriendlyName: strp("Phones")}},
		acls:      []twapi.ApiV2010SipIpAccessControlList{{Sid: strp("AL1"), FriendlyName: strp("Office")}},
		credMaps:  map[string][]twapi.ApiV2010SipCredentialListMapping{"SD1": {{FriendlyName: strp("Phones")}}},
		aclMaps:   map[string][]twapi.ApiV2010SipIpAccessControlListMapping{"SD1": {{FriendlyName: strp("Office")}}},
	}
	domains := &fakeSIPDomains{existing: []voiceml.SIPDomain{{Sid: "SDx", DomainName: "acme.vml.voice.tel"}}}
	credLists := &fakeSIPCredLists{existing: []voiceml.SIPCredentialList{{Sid: "CLx", FriendlyName: strp("Phones")}}}
	acls := &fakeSIPACLs{existing: []voiceml.SIPIpAccessControlList{{Sid: "ALx", FriendlyName: strp("Office")}}}

	res, err := migrateSIP(context.Background(), src, domains, credLists, acls, Options{DryRun: true})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Count(StatusPlanned) == 0 {
		t.Errorf("expected planned mapping items, got %+v", res.Items)
	}
}
