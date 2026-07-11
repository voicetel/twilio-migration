package migrate

import (
	"context"
	"fmt"

	twapi "github.com/twilio/twilio-go/rest/api/v2010"
	twvoice "github.com/twilio/twilio-go/rest/voice/v1"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

// byocTrunkSource is the slice of twilio-go used to read BYOC Trunks and the
// Connection Policies they reference (for SID→friendly-name resolution).
type byocTrunkSource interface {
	ListByocTrunk(params *twvoice.ListByocTrunkParams) ([]twvoice.VoiceV1ByocTrunk, error)
	ListConnectionPolicy(params *twvoice.ListConnectionPolicyParams) ([]twvoice.VoiceV1ConnectionPolicy, error)
}

// byocDomainSource is the slice of twilio-go used to resolve a BYOC Trunk's
// FromDomainSid to a domain name.
type byocDomainSource interface {
	ListSipDomain(params *twapi.ListSipDomainParams) ([]twapi.ApiV2010SipDomain, error)
}

// byocTrunkDest is the slice of voiceml-go-sdk used to read/write BYOC Trunks
// and to look up the (already-migrated) Connection Policies they reference.
type byocTrunkDest interface {
	ListByocTrunks(ctx context.Context, params voiceml.V1PageParams) (*voiceml.VoiceV1ByocTrunkList, error)
	CreateByocTrunk(ctx context.Context, params voiceml.CreateVoiceV1ByocTrunkParams) (*voiceml.VoiceV1ByocTrunk, error)
	ListConnectionPolicies(ctx context.Context, params voiceml.V1PageParams) (*voiceml.VoiceV1ConnectionPolicyList, error)
}

// byocDomainDest is the slice of voiceml-go-sdk used to look up the
// (already-migrated) SIP domains a BYOC Trunk's FromDomainSid references.
type byocDomainDest interface {
	List(ctx context.Context, params voiceml.ListPageParams) (*voiceml.SIPDomainList, error)
}

// ByocTrunks migrates Voice v1 BYOC (Bring Your Own Carrier) Trunks.
// Idempotent by friendly name. References a Connection Policy and,
// optionally, a SIP Domain — both are re-pointed at the corresponding
// already-migrated VoiceML resource (resolved by friendly name / domain
// name, since the source SIDs are meaningless on VoiceML). Must therefore
// run after connection-policies (G2) and sip-trunking.
type ByocTrunks struct{}

// Name implements Migrator.
func (ByocTrunks) Name() string { return "byoc-trunks" }

// Migrate implements Migrator.
func (ByocTrunks) Migrate(ctx context.Context, c *Clients, opts Options) (Result, error) {
	return migrateByocTrunks(ctx, c.TwilioVoice, c.Twilio, c.VoiceML.VoiceV1, c.VoiceML.SIP.Domains, opts)
}

func migrateByocTrunks(ctx context.Context, src byocTrunkSource, domainSrc byocDomainSource, dst byocTrunkDest, domainDst byocDomainDest, opts Options) (Result, error) {
	res := Result{Resource: "byoc-trunks"}

	trunks, err := src.ListByocTrunk(&twvoice.ListByocTrunkParams{})
	if err != nil {
		return res, fmt.Errorf("list Twilio BYOC trunks: %w", err)
	}

	existingTrunks, err := dst.ListByocTrunks(ctx, voiceml.V1PageParams{})
	if err != nil {
		return res, fmt.Errorf("list VoiceML BYOC trunks: %w", err)
	}
	have := make(map[string]bool, len(existingTrunks.ByocTrunks))
	for _, t := range existingTrunks.ByocTrunks {
		have[deref(t.FriendlyName)] = true
	}

	newPolicySIDByOldSID, err := byocConnectionPolicySIDMap(ctx, src, dst)
	if err != nil {
		return res, err
	}
	newDomainSIDByOldSID, err := byocDomainSIDMap(ctx, domainSrc, domainDst)
	if err != nil {
		return res, err
	}

	for _, t := range trunks {
		name := deref(t.FriendlyName)
		item := ItemResult{ID: name}

		switch {
		case name == "":
			item.Status = StatusFailed
			item.Detail = "source BYOC trunk has no friendly_name"
		case have[name]:
			item.Status = StatusSkipped
			item.Detail = "already present on VoiceML"
		case opts.DryRun:
			item.Status = StatusPlanned
		default:
			params := voiceml.CreateVoiceV1ByocTrunkParams{
				FriendlyName:         t.FriendlyName,
				VoiceURL:             t.VoiceUrl,
				VoiceMethod:          t.VoiceMethod,
				VoiceFallbackURL:     t.VoiceFallbackUrl,
				VoiceFallbackMethod:  t.VoiceFallbackMethod,
				StatusCallbackURL:    t.StatusCallbackUrl,
				StatusCallbackMethod: t.StatusCallbackMethod,
				CnamLookupEnabled:    t.CnamLookupEnabled,
			}

			if oldSID := deref(t.ConnectionPolicySid); oldSID != "" {
				newSID, ok := newPolicySIDByOldSID[oldSID]
				if !ok {
					item.Status = StatusFailed
					item.Detail = "no migrated connection policy for source SID " + oldSID
					res.Items = append(res.Items, item)
					continue
				}
				params.ConnectionPolicySid = &newSID
			}

			if oldSID := deref(t.FromDomainSid); oldSID != "" {
				newSID, ok := newDomainSIDByOldSID[oldSID]
				if !ok {
					item.Status = StatusFailed
					item.Detail = "no migrated SIP domain for source SID " + oldSID
					res.Items = append(res.Items, item)
					continue
				}
				params.FromDomainSid = &newSID
			}

			if _, cErr := dst.CreateByocTrunk(ctx, params); cErr != nil {
				item.Status = StatusFailed
				item.Detail = cErr.Error()
			} else {
				item.Status = StatusCreated
			}
		}

		res.Items = append(res.Items, item)
	}

	return res, nil
}

// byocConnectionPolicySIDMap resolves old (Twilio) connection policy SIDs to
// new (VoiceML) ones by bridging through the friendly name they share.
func byocConnectionPolicySIDMap(ctx context.Context, src byocTrunkSource, dst byocTrunkDest) (map[string]string, error) {
	twilioPolicies, err := src.ListConnectionPolicy(&twvoice.ListConnectionPolicyParams{})
	if err != nil {
		return nil, fmt.Errorf("list Twilio connection policies: %w", err)
	}
	voicemlPolicies, err := dst.ListConnectionPolicies(ctx, voiceml.V1PageParams{})
	if err != nil {
		return nil, fmt.Errorf("list VoiceML connection policies: %w", err)
	}

	newSIDByName := make(map[string]string, len(voicemlPolicies.ConnectionPolicies))
	for _, p := range voicemlPolicies.ConnectionPolicies {
		newSIDByName[deref(p.FriendlyName)] = deref(p.Sid)
	}

	newSIDByOldSID := make(map[string]string, len(twilioPolicies))
	for _, p := range twilioPolicies {
		if newSID, ok := newSIDByName[deref(p.FriendlyName)]; ok {
			newSIDByOldSID[deref(p.Sid)] = newSID
		}
	}
	return newSIDByOldSID, nil
}

// byocDomainSIDMap resolves old (Twilio) SIP domain SIDs to new (VoiceML)
// ones by bridging through the domain name they share.
func byocDomainSIDMap(ctx context.Context, src byocDomainSource, dst byocDomainDest) (map[string]string, error) {
	twilioDomains, err := src.ListSipDomain(&twapi.ListSipDomainParams{})
	if err != nil {
		return nil, fmt.Errorf("list Twilio SIP domains: %w", err)
	}
	voicemlDomains, err := dst.List(ctx, voiceml.ListPageParams{})
	if err != nil {
		return nil, fmt.Errorf("list VoiceML SIP domains: %w", err)
	}

	newSIDByName := make(map[string]string, len(voicemlDomains.Domains))
	for _, d := range voicemlDomains.Domains {
		newSIDByName[d.DomainName] = d.Sid
	}

	newSIDByOldSID := make(map[string]string, len(twilioDomains))
	for _, d := range twilioDomains {
		if newSID, ok := newSIDByName[deref(d.DomainName)]; ok {
			newSIDByOldSID[deref(d.Sid)] = newSID
		}
	}
	return newSIDByOldSID, nil
}
