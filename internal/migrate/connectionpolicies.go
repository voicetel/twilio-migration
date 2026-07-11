package migrate

import (
	"context"
	"fmt"

	twvoice "github.com/twilio/twilio-go/rest/voice/v1"
	voiceml "github.com/voicetel/voiceml-go-sdk"
)

// connPolicySource is the slice of twilio-go used to read Voice v1
// Connection Policies and their Targets.
type connPolicySource interface {
	ListConnectionPolicy(params *twvoice.ListConnectionPolicyParams) ([]twvoice.VoiceV1ConnectionPolicy, error)
	ListConnectionPolicyTarget(connectionPolicySid string, params *twvoice.ListConnectionPolicyTargetParams) ([]twvoice.VoiceV1ConnectionPolicyTarget, error)
}

// connPolicyDest is the slice of voiceml-go-sdk used to read/write
// Connection Policies and their nested Targets.
type connPolicyDest interface {
	ListConnectionPolicies(ctx context.Context, params voiceml.V1PageParams) (*voiceml.VoiceV1ConnectionPolicyList, error)
	CreateConnectionPolicy(ctx context.Context, params voiceml.CreateVoiceV1ConnectionPolicyParams) (*voiceml.VoiceV1ConnectionPolicy, error)
	ListConnectionPolicyTargets(ctx context.Context, connectionPolicySid string, params voiceml.V1PageParams) (*voiceml.VoiceV1ConnectionPolicyTargetList, error)
	CreateConnectionPolicyTarget(ctx context.Context, connectionPolicySid string, params voiceml.CreateVoiceV1ConnectionPolicyTargetParams) (*voiceml.VoiceV1ConnectionPolicyTarget, error)
}

// ConnectionPolicies migrates Voice v1 Connection Policies (named bags of SIP
// URI Targets) and their nested Targets. Policies are idempotent by friendly
// name; Targets are idempotent by SIP URI within their policy. Produces SIDs
// a future byoc-trunks migrator (G3) will need to re-point, so this runs
// before any resource that references a connection policy.
type ConnectionPolicies struct{}

// Name implements Migrator.
func (ConnectionPolicies) Name() string { return "connection-policies" }

// Migrate implements Migrator.
func (ConnectionPolicies) Migrate(ctx context.Context, c *Clients, opts Options) (Result, error) {
	return migrateConnectionPolicies(ctx, c.TwilioVoice, c.VoiceML.VoiceV1, opts)
}

func migrateConnectionPolicies(ctx context.Context, src connPolicySource, dst connPolicyDest, opts Options) (Result, error) {
	res := Result{Resource: "connection-policies"}

	policies, err := src.ListConnectionPolicy(&twvoice.ListConnectionPolicyParams{})
	if err != nil {
		return res, fmt.Errorf("list Twilio connection policies: %w", err)
	}

	existing, err := dst.ListConnectionPolicies(ctx, voiceml.V1PageParams{})
	if err != nil {
		return res, fmt.Errorf("list VoiceML connection policies: %w", err)
	}
	byName := make(map[string]string, len(existing.ConnectionPolicies))
	for _, p := range existing.ConnectionPolicies {
		byName[deref(p.FriendlyName)] = deref(p.Sid)
	}

	for _, p := range policies {
		name := deref(p.FriendlyName)
		newSID, ok := byName[name]
		item := ItemResult{ID: name}

		switch {
		case name == "":
			item.Status = StatusFailed
			item.Detail = "source connection policy has no friendly_name"
			res.Items = append(res.Items, item)
			continue
		case ok:
			item.Status = StatusSkipped
			item.Detail = "already present on VoiceML"
		case opts.DryRun:
			item.Status = StatusPlanned
		default:
			created, cErr := dst.CreateConnectionPolicy(ctx, voiceml.CreateVoiceV1ConnectionPolicyParams{FriendlyName: &name})
			if cErr != nil {
				item.Status = StatusFailed
				item.Detail = cErr.Error()
				res.Items = append(res.Items, item)
				continue
			}
			newSID = deref(created.Sid)
			byName[name] = newSID
			item.Status = StatusCreated
		}
		res.Items = append(res.Items, item)

		if newSID != "" {
			if err := migrateConnectionPolicyTargets(ctx, src, dst, deref(p.Sid), name, newSID, opts, &res); err != nil {
				return res, err
			}
		}
	}

	return res, nil
}

// migrateConnectionPolicyTargets copies the Targets of one Twilio connection
// policy into the corresponding VoiceML policy (voicemlPolicySid). Idempotent
// by SIP URI target string.
func migrateConnectionPolicyTargets(ctx context.Context, src connPolicySource, dst connPolicyDest, twilioPolicySid, policyLabel, voicemlPolicySid string, opts Options, res *Result) error {
	targets, err := src.ListConnectionPolicyTarget(twilioPolicySid, &twvoice.ListConnectionPolicyTargetParams{})
	if err != nil {
		return fmt.Errorf("list Twilio connection policy targets for %s: %w", policyLabel, err)
	}
	existing, err := dst.ListConnectionPolicyTargets(ctx, voicemlPolicySid, voiceml.V1PageParams{})
	if err != nil {
		return fmt.Errorf("list VoiceML connection policy targets: %w", err)
	}
	have := make(map[string]bool, len(existing.Targets))
	for _, t := range existing.Targets {
		have[deref(t.Target)] = true
	}

	for _, t := range targets {
		uri := deref(t.Target)
		id := "target " + uri + " (" + policyLabel + ")"

		switch {
		case uri == "":
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusFailed, Detail: "source target has no target URI"})
		case have[uri]:
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusSkipped, Detail: "already present on VoiceML"})
		case opts.DryRun:
			res.Items = append(res.Items, ItemResult{ID: id, Status: StatusPlanned})
		default:
			params := voiceml.CreateVoiceV1ConnectionPolicyTargetParams{
				Target:       uri,
				FriendlyName: t.FriendlyName,
				Enabled:      t.Enabled,
			}
			if t.Priority > 0 {
				pr := t.Priority
				params.Priority = &pr
			}
			if t.Weight > 0 {
				w := t.Weight
				params.Weight = &w
			}
			if _, cErr := dst.CreateConnectionPolicyTarget(ctx, voicemlPolicySid, params); cErr != nil {
				res.Items = append(res.Items, ItemResult{ID: id, Status: StatusFailed, Detail: cErr.Error()})
			} else {
				res.Items = append(res.Items, ItemResult{ID: id, Status: StatusCreated})
			}
		}
	}

	return nil
}
