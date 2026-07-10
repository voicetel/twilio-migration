package migrate

import (
	"context"
	"fmt"

	twapi "github.com/twilio/twilio-go/rest/api/v2010"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

// sipSource is the slice of twilio-go used to read SIP trunking config.
type sipSource interface {
	ListSipDomain(params *twapi.ListSipDomainParams) ([]twapi.ApiV2010SipDomain, error)
	ListSipCredentialList(params *twapi.ListSipCredentialListParams) ([]twapi.ApiV2010SipCredentialList, error)
	ListSipCredential(credentialListSid string, params *twapi.ListSipCredentialParams) ([]twapi.ApiV2010SipCredential, error)
	ListSipIpAccessControlList(params *twapi.ListSipIpAccessControlListParams) ([]twapi.ApiV2010SipIpAccessControlList, error)
	ListSipIpAddress(ipAccessControlListSid string, params *twapi.ListSipIpAddressParams) ([]twapi.ApiV2010SipIpAddress, error)
	ListSipCredentialListMapping(domainSid string, params *twapi.ListSipCredentialListMappingParams) ([]twapi.ApiV2010SipCredentialListMapping, error)
	ListSipIpAccessControlListMapping(domainSid string, params *twapi.ListSipIpAccessControlListMappingParams) ([]twapi.ApiV2010SipIpAccessControlListMapping, error)
}

type sipDomainsDest interface {
	List(ctx context.Context, params voiceml.ListPageParams) (*voiceml.SIPDomainList, error)
	Create(ctx context.Context, params voiceml.CreateSIPDomainParams) (*voiceml.SIPDomain, error)
	ListCredentialListMappings(ctx context.Context, domainSid string, params voiceml.ListPageParams) (*voiceml.SIPCredentialListMappingList, error)
	CreateCredentialListMapping(ctx context.Context, domainSid string, params voiceml.CreateSIPCredentialListMappingParams) (*voiceml.SIPDomainMapping, error)
	ListIpAccessControlListMappings(ctx context.Context, domainSid string, params voiceml.ListPageParams) (*voiceml.SIPIpAccessControlListMappingList, error)
	CreateIpAccessControlListMapping(ctx context.Context, domainSid string, params voiceml.CreateSIPIpAccessControlListMappingParams) (*voiceml.SIPDomainMapping, error)
}

type sipCredListsDest interface {
	List(ctx context.Context, params voiceml.ListPageParams) (*voiceml.SIPCredentialListList, error)
	Create(ctx context.Context, params voiceml.CreateSIPCredentialListParams) (*voiceml.SIPCredentialList, error)
	ListCredentials(ctx context.Context, credentialListSid string, params voiceml.ListPageParams) (*voiceml.SIPCredentialPage, error)
	CreateCredential(ctx context.Context, credentialListSid string, params voiceml.CreateSIPCredentialParams) (*voiceml.SIPCredential, error)
}

type sipACLsDest interface {
	List(ctx context.Context, params voiceml.ListPageParams) (*voiceml.SIPIpAccessControlListList, error)
	Create(ctx context.Context, params voiceml.CreateSIPIpAccessControlListParams) (*voiceml.SIPIpAccessControlList, error)
	ListIpAddresses(ctx context.Context, aclSid string, params voiceml.ListPageParams) (*voiceml.SIPIpAddressList, error)
	CreateIpAddress(ctx context.Context, aclSid string, params voiceml.CreateSIPIpAddressParams) (*voiceml.SIPIpAddress, error)
}

// SIP migrates the full SIP-trunking graph: credential lists (+ credentials),
// IP access control lists (+ IP addresses), domains, and the domain↔list and
// domain↔ACL mappings that bind them.
//
// CREDENTIAL PASSWORDS ARE NEVER COPIED. Twilio does not expose a credential's
// password, so this tool cannot read it. Each migrated credential is created on
// VoiceML with a brand-new, freshly generated password, which is reported so
// the operator can redistribute it to devices. Nothing device-side keeps
// working until the new password is distributed.
type SIP struct{}

// Name implements Migrator.
func (SIP) Name() string { return "sip-trunking" }

// Migrate implements Migrator.
func (SIP) Migrate(ctx context.Context, c *Clients, opts Options) (Result, error) {
	return migrateSIP(ctx, c.Twilio, c.VoiceML.SIP.Domains, c.VoiceML.SIP.CredentialLists, c.VoiceML.SIP.IpAccessControlLists, opts)
}

// sipRun carries the shared state for one SIP migration: the accumulating
// result, the source/dest handles, and the friendly-name→new-SID maps used to
// re-point mappings at the freshly created VoiceML resources.
type sipRun struct {
	ctx       context.Context
	src       sipSource
	domains   sipDomainsDest
	credLists sipCredListsDest
	acls      sipACLsDest
	opts      Options
	res       *Result

	credListByName map[string]string // friendly name → new VoiceML cred-list SID
	aclByName      map[string]string // friendly name → new VoiceML ACL SID
	domainBySID    map[string]string // domain name → new VoiceML domain SID
}

func migrateSIP(ctx context.Context, src sipSource, domains sipDomainsDest, credLists sipCredListsDest, acls sipACLsDest, opts Options) (Result, error) {
	res := Result{Resource: "sip-trunking"}
	r := &sipRun{
		ctx: ctx, src: src, domains: domains, credLists: credLists, acls: acls, opts: opts, res: &res,
		credListByName: map[string]string{},
		aclByName:      map[string]string{},
		domainBySID:    map[string]string{},
	}

	if err := r.migrateCredentialLists(); err != nil {
		return res, err
	}
	if err := r.migrateAccessControlLists(); err != nil {
		return res, err
	}
	if err := r.migrateDomains(); err != nil {
		return res, err
	}
	if err := r.migrateMappings(); err != nil {
		return res, err
	}

	return res, nil
}

func (r *sipRun) add(id string, status ItemStatus, detail string) {
	r.res.Items = append(r.res.Items, ItemResult{ID: id, Status: status, Detail: detail})
}

// migrateCredentialLists creates each credential list (idempotent by friendly
// name), records the name→new-SID map, then migrates each list's credentials
// with freshly generated passwords.
func (r *sipRun) migrateCredentialLists() error {
	lists, err := r.src.ListSipCredentialList(&twapi.ListSipCredentialListParams{})
	if err != nil {
		return fmt.Errorf("list Twilio SIP credential lists: %w", err)
	}
	existing, err := r.credLists.List(r.ctx, voiceml.ListPageParams{})
	if err != nil {
		return fmt.Errorf("list VoiceML SIP credential lists: %w", err)
	}
	for _, l := range existing.CredentialLists {
		r.credListByName[deref(l.FriendlyName)] = l.Sid
	}

	for _, l := range lists {
		name := deref(l.FriendlyName)
		newSID, ok := r.credListByName[name]
		switch {
		case name == "":
			r.add("credential-list", StatusFailed, "source credential list has no friendly_name")
			continue
		case ok:
			r.add("credential-list "+name, StatusSkipped, "already present on VoiceML")
		case r.opts.DryRun:
			r.add("credential-list "+name, StatusPlanned, "")
		default:
			created, cErr := r.credLists.Create(r.ctx, voiceml.CreateSIPCredentialListParams{FriendlyName: name})
			if cErr != nil {
				r.add("credential-list "+name, StatusFailed, cErr.Error())
				continue
			}
			newSID = created.Sid
			r.credListByName[name] = newSID
			r.add("credential-list "+name, StatusCreated, "")
		}

		// Migrate the credentials in this Twilio list into the corresponding
		// VoiceML list (newSID), when we have one.
		if newSID != "" {
			if err := r.migrateCredentials(deref(l.Sid), name, newSID); err != nil {
				return err
			}
		}
	}

	return nil
}

// migrateCredentials copies the usernames from a Twilio credential list into the
// VoiceML list, minting a NEW password for each (Twilio never returns the
// original). Idempotent by username. The generated password is reported.
func (r *sipRun) migrateCredentials(twilioListSid, twilioListLabel, voicemlListSid string) error {
	creds, err := r.src.ListSipCredential(twilioListSid, &twapi.ListSipCredentialParams{})
	if err != nil {
		return fmt.Errorf("list Twilio SIP credentials for %s: %w", twilioListLabel, err)
	}
	existing, err := r.credLists.ListCredentials(r.ctx, voicemlListSid, voiceml.ListPageParams{})
	if err != nil {
		return fmt.Errorf("list VoiceML SIP credentials: %w", err)
	}
	have := make(map[string]bool, len(existing.Credentials))
	for _, c := range existing.Credentials {
		have[c.Username] = true
	}

	for _, cr := range creds {
		username := deref(cr.Username)
		id := "credential " + username
		switch {
		case username == "":
			r.add(id, StatusFailed, "source credential has no username")
		case have[username]:
			r.add(id, StatusSkipped, "already present on VoiceML")
		case r.opts.DryRun:
			r.add(id, StatusPlanned, "password will be generated (Twilio does not expose the original)")
		default:
			pw, gErr := generatePassword()
			if gErr != nil {
				r.add(id, StatusFailed, gErr.Error())
				continue
			}
			if _, cErr := r.credLists.CreateCredential(r.ctx, voicemlListSid, voiceml.CreateSIPCredentialParams{
				Username: username,
				Password: pw,
			}); cErr != nil {
				r.add(id, StatusFailed, cErr.Error())
				continue
			}
			r.add(id, StatusCreated, "NEW generated password (redistribute to device): "+pw)
		}
	}

	return nil
}

// migrateAccessControlLists creates each IP ACL (idempotent by friendly name),
// records the name→new-SID map, then migrates each ACL's IP addresses.
func (r *sipRun) migrateAccessControlLists() error {
	acls, err := r.src.ListSipIpAccessControlList(&twapi.ListSipIpAccessControlListParams{})
	if err != nil {
		return fmt.Errorf("list Twilio SIP IP ACLs: %w", err)
	}
	existing, err := r.acls.List(r.ctx, voiceml.ListPageParams{})
	if err != nil {
		return fmt.Errorf("list VoiceML SIP IP ACLs: %w", err)
	}
	for _, a := range existing.IpAccessControlLists {
		r.aclByName[deref(a.FriendlyName)] = a.Sid
	}

	for _, a := range acls {
		name := deref(a.FriendlyName)
		newSID, ok := r.aclByName[name]
		switch {
		case name == "":
			r.add("ip-acl", StatusFailed, "source IP ACL has no friendly_name")
			continue
		case ok:
			r.add("ip-acl "+name, StatusSkipped, "already present on VoiceML")
		case r.opts.DryRun:
			r.add("ip-acl "+name, StatusPlanned, "")
		default:
			created, cErr := r.acls.Create(r.ctx, voiceml.CreateSIPIpAccessControlListParams{FriendlyName: name})
			if cErr != nil {
				r.add("ip-acl "+name, StatusFailed, cErr.Error())
				continue
			}
			newSID = created.Sid
			r.aclByName[name] = newSID
			r.add("ip-acl "+name, StatusCreated, "")
		}

		if newSID != "" {
			if err := r.migrateIPAddresses(deref(a.Sid), name, newSID); err != nil {
				return err
			}
		}
	}

	return nil
}

// migrateIPAddresses copies the IP entries of a Twilio ACL into the VoiceML ACL.
// Idempotent by IP address.
func (r *sipRun) migrateIPAddresses(twilioACLSid, aclLabel, voicemlACLSid string) error {
	addrs, err := r.src.ListSipIpAddress(twilioACLSid, &twapi.ListSipIpAddressParams{})
	if err != nil {
		return fmt.Errorf("list Twilio SIP IP addresses for %s: %w", aclLabel, err)
	}
	existing, err := r.acls.ListIpAddresses(r.ctx, voicemlACLSid, voiceml.ListPageParams{})
	if err != nil {
		return fmt.Errorf("list VoiceML SIP IP addresses: %w", err)
	}
	have := make(map[string]bool, len(existing.IpAddresses))
	for _, ip := range existing.IpAddresses {
		have[ip.IpAddress] = true
	}

	for _, ip := range addrs {
		addr := deref(ip.IpAddress)
		id := "ip " + addr + " (" + aclLabel + ")"
		switch {
		case addr == "":
			r.add(id, StatusFailed, "source IP address is empty")
		case have[addr]:
			r.add(id, StatusSkipped, "already present on VoiceML")
		case r.opts.DryRun:
			r.add(id, StatusPlanned, "")
		default:
			params := voiceml.CreateSIPIpAddressParams{
				FriendlyName: deref(ip.FriendlyName),
				IpAddress:    addr,
			}
			if ip.CidrPrefixLength > 0 {
				cidr := ip.CidrPrefixLength
				params.CidrPrefixLength = &cidr
			}
			if _, cErr := r.acls.CreateIpAddress(r.ctx, voicemlACLSid, params); cErr != nil {
				r.add(id, StatusFailed, cErr.Error())
			} else {
				r.add(id, StatusCreated, "")
			}
		}
	}

	return nil
}

// migrateDomains creates each SIP domain (idempotent by domain name) and records
// the name→new-SID map for the mapping phase.
func (r *sipRun) migrateDomains() error {
	domains, err := r.src.ListSipDomain(&twapi.ListSipDomainParams{})
	if err != nil {
		return fmt.Errorf("list Twilio SIP domains: %w", err)
	}
	existing, err := r.domains.List(r.ctx, voiceml.ListPageParams{})
	if err != nil {
		return fmt.Errorf("list VoiceML SIP domains: %w", err)
	}
	for _, d := range existing.Domains {
		r.domainBySID[d.DomainName] = d.Sid
	}

	for _, d := range domains {
		name := deref(d.DomainName)
		id := "domain " + name
		switch {
		case name == "":
			r.add("domain", StatusFailed, "source domain has no domain_name")
		case r.domainBySID[name] != "":
			r.add(id, StatusSkipped, "already present on VoiceML")
		case r.opts.DryRun:
			r.add(id, StatusPlanned, "")
		default:
			created, cErr := r.domains.Create(r.ctx, voiceml.CreateSIPDomainParams{
				DomainName:   name,
				FriendlyName: d.FriendlyName,
				VoiceURL:     d.VoiceUrl,
				VoiceMethod:  d.VoiceMethod,
			})
			if cErr != nil {
				r.add(id, StatusFailed, cErr.Error())
				continue
			}
			r.domainBySID[name] = created.Sid
			r.add(id, StatusCreated, "")
		}
	}

	return nil
}

// migrateMappings re-creates each domain's credential-list and IP-ACL mappings,
// re-pointing them at the freshly created VoiceML resources (matched by the
// friendly names captured earlier).
func (r *sipRun) migrateMappings() error {
	domains, err := r.src.ListSipDomain(&twapi.ListSipDomainParams{})
	if err != nil {
		return fmt.Errorf("list Twilio SIP domains (mappings): %w", err)
	}

	for _, d := range domains {
		domainName := deref(d.DomainName)
		newDomainSID := r.domainBySID[domainName]
		if newDomainSID == "" {
			// Domain wasn't created (e.g. dry-run or a create failure); nothing
			// to attach mappings to.
			continue
		}

		if err := r.migrateCredListMappings(deref(d.Sid), domainName, newDomainSID); err != nil {
			return err
		}
		if err := r.migrateACLMappings(deref(d.Sid), domainName, newDomainSID); err != nil {
			return err
		}
	}

	return nil
}

func (r *sipRun) migrateCredListMappings(twilioDomainSid, domainName, newDomainSID string) error {
	maps, err := r.src.ListSipCredentialListMapping(twilioDomainSid, &twapi.ListSipCredentialListMappingParams{})
	if err != nil {
		return fmt.Errorf("list Twilio credential-list mappings for %s: %w", domainName, err)
	}
	existing, err := r.domains.ListCredentialListMappings(r.ctx, newDomainSID, voiceml.ListPageParams{})
	if err != nil {
		return fmt.Errorf("list VoiceML credential-list mappings: %w", err)
	}
	mapped := make(map[string]bool, len(existing.CredentialListMappings))
	for _, m := range existing.CredentialListMappings {
		mapped[m.Sid] = true
	}

	for _, m := range maps {
		name := deref(m.FriendlyName)
		id := "cred-list-mapping " + name + " -> " + domainName
		newListSID, ok := r.credListByName[name]
		switch {
		case !ok:
			r.add(id, StatusFailed, "no migrated credential list named "+name)
		case mapped[newListSID]:
			r.add(id, StatusSkipped, "already mapped on VoiceML")
		case r.opts.DryRun:
			r.add(id, StatusPlanned, "")
		default:
			if _, cErr := r.domains.CreateCredentialListMapping(r.ctx, newDomainSID, voiceml.CreateSIPCredentialListMappingParams{CredentialListSid: newListSID}); cErr != nil {
				r.add(id, StatusFailed, cErr.Error())
			} else {
				r.add(id, StatusCreated, "")
			}
		}
	}

	return nil
}

func (r *sipRun) migrateACLMappings(twilioDomainSid, domainName, newDomainSID string) error {
	maps, err := r.src.ListSipIpAccessControlListMapping(twilioDomainSid, &twapi.ListSipIpAccessControlListMappingParams{})
	if err != nil {
		return fmt.Errorf("list Twilio IP-ACL mappings for %s: %w", domainName, err)
	}
	existing, err := r.domains.ListIpAccessControlListMappings(r.ctx, newDomainSID, voiceml.ListPageParams{})
	if err != nil {
		return fmt.Errorf("list VoiceML IP-ACL mappings: %w", err)
	}
	mapped := make(map[string]bool, len(existing.IpAccessControlListMappings))
	for _, m := range existing.IpAccessControlListMappings {
		mapped[m.Sid] = true
	}

	for _, m := range maps {
		name := deref(m.FriendlyName)
		id := "ip-acl-mapping " + name + " -> " + domainName
		newACLSID, ok := r.aclByName[name]
		switch {
		case !ok:
			r.add(id, StatusFailed, "no migrated IP ACL named "+name)
		case mapped[newACLSID]:
			r.add(id, StatusSkipped, "already mapped on VoiceML")
		case r.opts.DryRun:
			r.add(id, StatusPlanned, "")
		default:
			if _, cErr := r.domains.CreateIpAccessControlListMapping(r.ctx, newDomainSID, voiceml.CreateSIPIpAccessControlListMappingParams{IpAccessControlListSid: newACLSID}); cErr != nil {
				r.add(id, StatusFailed, cErr.Error())
			} else {
				r.add(id, StatusCreated, "")
			}
		}
	}

	return nil
}
