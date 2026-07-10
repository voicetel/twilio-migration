package migrate

import (
	"context"
	"strings"
	"testing"

	twapi "github.com/twilio/twilio-go/rest/api/v2010"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

// --- fakes ---

type fakeSIPSource struct {
	domains   []twapi.ApiV2010SipDomain
	credLists []twapi.ApiV2010SipCredentialList
	creds     map[string][]twapi.ApiV2010SipCredential // keyed by cred-list sid
	acls      []twapi.ApiV2010SipIpAccessControlList
	ips       map[string][]twapi.ApiV2010SipIpAddress // keyed by acl sid
	credMaps  map[string][]twapi.ApiV2010SipCredentialListMapping
	aclMaps   map[string][]twapi.ApiV2010SipIpAccessControlListMapping
}

func (f fakeSIPSource) ListSipDomain(*twapi.ListSipDomainParams) ([]twapi.ApiV2010SipDomain, error) {
	return f.domains, nil
}
func (f fakeSIPSource) ListSipCredentialList(*twapi.ListSipCredentialListParams) ([]twapi.ApiV2010SipCredentialList, error) {
	return f.credLists, nil
}
func (f fakeSIPSource) ListSipCredential(listSid string, _ *twapi.ListSipCredentialParams) ([]twapi.ApiV2010SipCredential, error) {
	return f.creds[listSid], nil
}
func (f fakeSIPSource) ListSipIpAccessControlList(*twapi.ListSipIpAccessControlListParams) ([]twapi.ApiV2010SipIpAccessControlList, error) {
	return f.acls, nil
}
func (f fakeSIPSource) ListSipIpAddress(aclSid string, _ *twapi.ListSipIpAddressParams) ([]twapi.ApiV2010SipIpAddress, error) {
	return f.ips[aclSid], nil
}
func (f fakeSIPSource) ListSipCredentialListMapping(domainSid string, _ *twapi.ListSipCredentialListMappingParams) ([]twapi.ApiV2010SipCredentialListMapping, error) {
	return f.credMaps[domainSid], nil
}
func (f fakeSIPSource) ListSipIpAccessControlListMapping(domainSid string, _ *twapi.ListSipIpAccessControlListMappingParams) ([]twapi.ApiV2010SipIpAccessControlListMapping, error) {
	return f.aclMaps[domainSid], nil
}

type fakeSIPCredLists struct {
	existing     []voiceml.SIPCredentialList
	existingCred []voiceml.SIPCredential
	created      []string // friendly names
	credentials  []voiceml.CreateSIPCredentialParams
	nextSID      int
}

func (f *fakeSIPCredLists) List(context.Context, voiceml.ListPageParams) (*voiceml.SIPCredentialListList, error) {
	return &voiceml.SIPCredentialListList{CredentialLists: f.existing}, nil
}
func (f *fakeSIPCredLists) Create(_ context.Context, p voiceml.CreateSIPCredentialListParams) (*voiceml.SIPCredentialList, error) {
	f.created = append(f.created, p.FriendlyName)
	f.nextSID++
	return &voiceml.SIPCredentialList{Sid: "CLnew", FriendlyName: strp(p.FriendlyName)}, nil
}
func (f *fakeSIPCredLists) ListCredentials(context.Context, string, voiceml.ListPageParams) (*voiceml.SIPCredentialPage, error) {
	return &voiceml.SIPCredentialPage{Credentials: f.existingCred}, nil
}
func (f *fakeSIPCredLists) CreateCredential(_ context.Context, _ string, p voiceml.CreateSIPCredentialParams) (*voiceml.SIPCredential, error) {
	f.credentials = append(f.credentials, p)
	return &voiceml.SIPCredential{Username: p.Username}, nil
}

type fakeSIPACLs struct {
	existing   []voiceml.SIPIpAccessControlList
	existingIP []voiceml.SIPIpAddress
	created    []string
	ips        []voiceml.CreateSIPIpAddressParams
}

func (f *fakeSIPACLs) List(context.Context, voiceml.ListPageParams) (*voiceml.SIPIpAccessControlListList, error) {
	return &voiceml.SIPIpAccessControlListList{IpAccessControlLists: f.existing}, nil
}
func (f *fakeSIPACLs) Create(_ context.Context, p voiceml.CreateSIPIpAccessControlListParams) (*voiceml.SIPIpAccessControlList, error) {
	f.created = append(f.created, p.FriendlyName)
	return &voiceml.SIPIpAccessControlList{Sid: "ALnew", FriendlyName: strp(p.FriendlyName)}, nil
}
func (f *fakeSIPACLs) ListIpAddresses(context.Context, string, voiceml.ListPageParams) (*voiceml.SIPIpAddressList, error) {
	return &voiceml.SIPIpAddressList{IpAddresses: f.existingIP}, nil
}
func (f *fakeSIPACLs) CreateIpAddress(_ context.Context, _ string, p voiceml.CreateSIPIpAddressParams) (*voiceml.SIPIpAddress, error) {
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
}

func (f *fakeSIPDomains) List(context.Context, voiceml.ListPageParams) (*voiceml.SIPDomainList, error) {
	return &voiceml.SIPDomainList{Domains: f.existing}, nil
}
func (f *fakeSIPDomains) Create(_ context.Context, p voiceml.CreateSIPDomainParams) (*voiceml.SIPDomain, error) {
	f.created = append(f.created, p.DomainName)
	return &voiceml.SIPDomain{Sid: "SDnew", DomainName: p.DomainName}, nil
}
func (f *fakeSIPDomains) ListCredentialListMappings(context.Context, string, voiceml.ListPageParams) (*voiceml.SIPCredentialListMappingList, error) {
	return &voiceml.SIPCredentialListMappingList{CredentialListMappings: f.existingCredMap}, nil
}
func (f *fakeSIPDomains) CreateCredentialListMapping(_ context.Context, _ string, p voiceml.CreateSIPCredentialListMappingParams) (*voiceml.SIPDomainMapping, error) {
	f.credMaps = append(f.credMaps, p.CredentialListSid)
	return &voiceml.SIPDomainMapping{}, nil
}
func (f *fakeSIPDomains) ListIpAccessControlListMappings(context.Context, string, voiceml.ListPageParams) (*voiceml.SIPIpAccessControlListMappingList, error) {
	return &voiceml.SIPIpAccessControlListMappingList{IpAccessControlListMappings: f.existingACLMap}, nil
}
func (f *fakeSIPDomains) CreateIpAccessControlListMapping(_ context.Context, _ string, p voiceml.CreateSIPIpAccessControlListMappingParams) (*voiceml.SIPDomainMapping, error) {
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
