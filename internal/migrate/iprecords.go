package migrate

import (
	"context"
	"fmt"

	twvoice "github.com/twilio/twilio-go/rest/voice/v1"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

// ipRecordSource is the slice of twilio-go used to read Voice v1 IP Records.
type ipRecordSource interface {
	ListIpRecord(params *twvoice.ListIpRecordParams) ([]twvoice.VoiceV1IpRecord, error)
}

// ipRecordDest is the slice of voiceml-go-sdk used to read/write IP Records.
type ipRecordDest interface {
	ListIpRecords(ctx context.Context, params voiceml.V1PageParams) (*voiceml.VoiceV1IpRecordList, error)
	CreateIpRecord(ctx context.Context, params voiceml.CreateVoiceV1IpRecordParams) (*voiceml.VoiceV1IpRecord, error)
}

// IPRecords migrates Voice v1 IP Records (standalone allowed source IPs).
// Idempotent by IP address. Standalone: does not itself reference any other
// resource, but a future source-ip-mappings migrator will need to resolve
// its old→new SID map, so this runs early in Default().
type IPRecords struct{}

// Name implements Migrator.
func (IPRecords) Name() string { return "ip-records" }

// Migrate implements Migrator.
func (IPRecords) Migrate(ctx context.Context, c *Clients, opts Options) (Result, error) {
	return migrateIPRecords(ctx, c.TwilioVoice, c.VoiceML.VoiceV1, opts)
}

func migrateIPRecords(ctx context.Context, src ipRecordSource, dst ipRecordDest, opts Options) (Result, error) {
	res := Result{Resource: "ip-records"}

	records, err := src.ListIpRecord(&twvoice.ListIpRecordParams{})
	if err != nil {
		return res, fmt.Errorf("list Twilio IP records: %w", err)
	}

	existing, err := dst.ListIpRecords(ctx, voiceml.V1PageParams{})
	if err != nil {
		return res, fmt.Errorf("list VoiceML IP records: %w", err)
	}

	have := make(map[string]bool, len(existing.IpRecords))
	for _, r := range existing.IpRecords {
		have[deref(r.IpAddress)] = true
	}

	for _, r := range records {
		addr := deref(r.IpAddress)
		item := ItemResult{ID: addr}

		switch {
		case addr == "":
			item.Status = StatusFailed
			item.Detail = "source IP record has no ip_address"
		case have[addr]:
			item.Status = StatusSkipped
			item.Detail = "already present on VoiceML"
		case opts.DryRun:
			item.Status = StatusPlanned
		default:
			params := voiceml.CreateVoiceV1IpRecordParams{
				IpAddress:    addr,
				FriendlyName: r.FriendlyName,
			}
			if r.CidrPrefixLength > 0 {
				cidr := r.CidrPrefixLength
				params.CidrPrefixLength = &cidr
			}
			if _, cErr := dst.CreateIpRecord(ctx, params); cErr != nil {
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
