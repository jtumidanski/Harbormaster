package policies

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	madmin "github.com/minio/madmin-go/v3"
)

// listCanned retrieves all canned policies from MinIO and converts them
// into the sorted []Policy slice that List returns. The sort is by name so
// the UI gets a deterministic order without depending on map iteration.
func listCanned(ctx context.Context, adm adminAPI) ([]Policy, error) {
	raw, err := adm.ListCannedPolicies(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Policy, 0, len(raw))
	for name, doc := range raw {
		out = append(out, policyFromEntry(name, doc))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// attachmentScan returns the users and groups that have policy name attached.
//
// User attachment: ListUsers and inspect UserInfo.PolicyName (comma-separated).
// Group attachment: ListGroups then GetGroupDescription for each group; check
// GroupDesc.Policy (comma-separated). Group enumeration is best-effort — if
// ListGroups errors we skip group scanning and return the users-only result
// with a nil error so a transient RPC failure never blocks a delete.
func attachmentScan(ctx context.Context, adm adminAPI, name string) (users, groups []string, err error) {
	rawUsers, err := adm.ListUsers(ctx)
	if err != nil {
		return nil, nil, err
	}
	for ak, info := range rawUsers {
		if containsPolicy(info.PolicyName, name) {
			users = append(users, ak)
		}
	}
	sort.Strings(users)

	groupNames, err := adm.ListGroups(ctx)
	if err != nil {
		//nolint:nilerr
		return users, nil, nil
	}
	for _, g := range groupNames {
		desc, err := adm.GetGroupDescription(ctx, g)
		if err != nil {
			// Best-effort: skip groups we cannot interrogate.
			continue
		}
		if containsPolicy(desc.Policy, name) {
			groups = append(groups, g)
		}
	}
	sort.Strings(groups)

	return users, groups, nil
}

// containsPolicy reports whether the comma-separated policy CSV contains the
// exact policy name. This handles both single-policy and multi-policy strings
// that MinIO stores in UserInfo.PolicyName and GroupDesc.Policy.
func containsPolicy(csv, name string) bool {
	if csv == "" || name == "" {
		return false
	}
	for _, part := range strings.Split(csv, ",") {
		if strings.TrimSpace(part) == name {
			return true
		}
	}
	return false
}

// adminAPI is the subset of *madmin.AdminClient the policies processor uses.
// Defining it as a local interface lets tests substitute an in-memory stub
// without standing up a fake MinIO server. The shape mirrors madmin v3's
// signatures verbatim so the live client satisfies the interface by
// structural typing.
type adminAPI interface {
	ListCannedPolicies(ctx context.Context) (map[string]json.RawMessage, error)
	InfoCannedPolicy(ctx context.Context, policyName string) ([]byte, error)
	AddCannedPolicy(ctx context.Context, policyName string, policy []byte) error
	RemoveCannedPolicy(ctx context.Context, policyName string) error
	ListUsers(ctx context.Context) (map[string]madmin.UserInfo, error)
	ListGroups(ctx context.Context) ([]string, error)
	GetGroupDescription(ctx context.Context, group string) (*madmin.GroupDesc, error)
}
