package migrate

import (
	"context"
	"fmt"

	twapi "github.com/twilio/twilio-go/rest/api/v2010"
	twvoice "github.com/twilio/twilio-go/rest/voice/v1"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

// sourceIPMappingSource is the slice of twilio-go used to read Voice v1
// Source IP Mappings and the IP Records they reference (for
// SID→IP-address resolution).
type sourceIPMappingSource interface {
	ListSourceIpMapping(params *twvoice.ListSourceIpMappingParams) ([]twvoice.VoiceV1SourceIpMapping, error)
	ListIpRecord(params *twvoice.ListIpRecordParams) ([]twvoice.VoiceV1IpRecord, error)
}

// sourceIPMappingDest is the slice of voiceml-go-sdk used to read/write
// Source IP Mappings and to look up the (already-migrated) IP Records they
// reference.
type sourceIPMappingDest interface {
	ListSourceIpMappings(ctx context.Context, params voiceml.V1PageParams) (*voiceml.VoiceV1SourceIpMappingList, error)
	CreateSourceIpMapping(ctx context.Context, params voiceml.CreateVoiceV1SourceIpMappingParams) (*voiceml.VoiceV1SourceIpMapping, error)
	ListIpRecords(ctx context.Context, params voiceml.V1PageParams) (*voiceml.VoiceV1IpRecordList, error)
}

// SourceIPMappings migrates Voice v1 Source IP Mappings, which bind an IP
// Record to a SIP Domain. Idempotent by the resolved (IP record, SIP domain)
// pair. Both references are re-pointed at the already-migrated VoiceML
// resources: the Twilio mapping only gives the OLD SIDs, which are
// meaningless on VoiceML, so each is bridged to its NEW SID via the natural
// key it shares across accounts (IP address for records, domain name for SIP
// domains). Must run after ip-records (G1) and sip-trunking.
type SourceIPMappings struct{}

// Name implements Migrator.
func (SourceIPMappings) Name() string { return "source-ip-mappings" }

// Migrate implements Migrator.
func (SourceIPMappings) Migrate(ctx context.Context, c *Clients, opts Options) (Result, error) {
	return migrateSourceIPMappings(ctx, c.TwilioVoice, c.Twilio, c.VoiceML.VoiceV1, c.VoiceML.SIP.Domains, opts)
}

func migrateSourceIPMappings(ctx context.Context, src sourceIPMappingSource, domainSrc byocDomainSource, dst sourceIPMappingDest, domainDst byocDomainDest, opts Options) (Result, error) {
	res := Result{Resource: "source-ip-mappings"}

	mappings, err := src.ListSourceIpMapping(&twvoice.ListSourceIpMappingParams{})
	if err != nil {
		return res, fmt.Errorf("list Twilio source IP mappings: %w", err)
	}

	newIPSIDByOldSID, ipLabelByOldSID, err := sourceIPRecordBridge(ctx, src, dst)
	if err != nil {
		return res, err
	}
	newDomainSIDByOldSID, domainLabelByOldSID, err := sourceDomainBridge(ctx, domainSrc, domainDst)
	if err != nil {
		return res, err
	}

	existing, err := dst.ListSourceIpMappings(ctx, voiceml.V1PageParams{})
	if err != nil {
		return res, fmt.Errorf("list VoiceML source IP mappings: %w", err)
	}
	have := make(map[string]bool, len(existing.SourceIpMappings))
	for _, m := range existing.SourceIpMappings {
		have[deref(m.IpRecordSid)+"|"+deref(m.SipDomainSid)] = true
	}

	for _, m := range mappings {
		oldIPSID := deref(m.IpRecordSid)
		oldDomainSID := deref(m.SipDomainSid)
		id := "ip-record " + labelOr(ipLabelByOldSID, oldIPSID) + " -> domain " + labelOr(domainLabelByOldSID, oldDomainSID)
		item := ItemResult{ID: id}

		newIPSID, ipOK := newIPSIDByOldSID[oldIPSID]
		newDomainSID, domainOK := newDomainSIDByOldSID[oldDomainSID]

		switch {
		case !ipOK:
			item.Status = StatusFailed
			item.Detail = "no migrated IP record for source SID " + oldIPSID
		case !domainOK:
			item.Status = StatusFailed
			item.Detail = "no migrated SIP domain for source SID " + oldDomainSID
		case have[newIPSID+"|"+newDomainSID]:
			item.Status = StatusSkipped
			item.Detail = "already present on VoiceML"
		case opts.DryRun:
			item.Status = StatusPlanned
		default:
			if _, cErr := dst.CreateSourceIpMapping(ctx, voiceml.CreateVoiceV1SourceIpMappingParams{
				IpRecordSid:  newIPSID,
				SipDomainSid: newDomainSID,
			}); cErr != nil {
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

// labelOr returns labels[sid], or sid itself if it has no known label (e.g.
// the mapping references a SID that no longer appears in Twilio's own IP
// record / SIP domain list — shouldn't happen, but keeps the item ID
// informative instead of empty).
func labelOr(labels map[string]string, sid string) string {
	if l, ok := labels[sid]; ok && l != "" {
		return l
	}
	return sid
}

// sourceIPRecordBridge resolves old (Twilio) IP record SIDs to new
// (VoiceML) ones by bridging through the IP address they share, and returns
// a human-readable label (the IP address) for each old SID.
func sourceIPRecordBridge(ctx context.Context, src sourceIPMappingSource, dst sourceIPMappingDest) (newSIDByOldSID, labelByOldSID map[string]string, err error) {
	twilioRecords, err := src.ListIpRecord(&twvoice.ListIpRecordParams{})
	if err != nil {
		return nil, nil, fmt.Errorf("list Twilio IP records: %w", err)
	}
	voicemlRecords, err := dst.ListIpRecords(ctx, voiceml.V1PageParams{})
	if err != nil {
		return nil, nil, fmt.Errorf("list VoiceML IP records: %w", err)
	}

	newSIDByAddr := make(map[string]string, len(voicemlRecords.IpRecords))
	for _, r := range voicemlRecords.IpRecords {
		newSIDByAddr[deref(r.IpAddress)] = deref(r.Sid)
	}

	newSIDByOldSID = make(map[string]string, len(twilioRecords))
	labelByOldSID = make(map[string]string, len(twilioRecords))
	for _, r := range twilioRecords {
		oldSID := deref(r.Sid)
		addr := deref(r.IpAddress)
		labelByOldSID[oldSID] = addr
		if newSID, ok := newSIDByAddr[addr]; ok {
			newSIDByOldSID[oldSID] = newSID
		}
	}
	return newSIDByOldSID, labelByOldSID, nil
}

// sourceDomainBridge resolves old (Twilio) SIP domain SIDs to new (VoiceML)
// ones by bridging through the domain name they share, and returns a
// human-readable label (the domain name) for each old SID.
func sourceDomainBridge(ctx context.Context, src byocDomainSource, dst byocDomainDest) (newSIDByOldSID, labelByOldSID map[string]string, err error) {
	twilioDomains, err := src.ListSipDomain(&twapi.ListSipDomainParams{})
	if err != nil {
		return nil, nil, fmt.Errorf("list Twilio SIP domains: %w", err)
	}
	voicemlDomains, err := dst.List(ctx, voiceml.ListPageParams{})
	if err != nil {
		return nil, nil, fmt.Errorf("list VoiceML SIP domains: %w", err)
	}

	newSIDByName := make(map[string]string, len(voicemlDomains.Domains))
	for _, d := range voicemlDomains.Domains {
		newSIDByName[d.DomainName] = d.Sid
	}

	newSIDByOldSID = make(map[string]string, len(twilioDomains))
	labelByOldSID = make(map[string]string, len(twilioDomains))
	for _, d := range twilioDomains {
		oldSID := deref(d.Sid)
		name := deref(d.DomainName)
		labelByOldSID[oldSID] = name
		if newSID, ok := newSIDByName[name]; ok {
			newSIDByOldSID[oldSID] = newSID
		}
	}
	return newSIDByOldSID, labelByOldSID, nil
}
