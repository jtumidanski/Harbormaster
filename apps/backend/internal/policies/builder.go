package policies

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

// policyNameRE is MinIO's canned-policy name charset: alphanumerics plus
// - _ . / — and 1..128 characters.
var policyNameRE = regexp.MustCompile(`^[A-Za-z0-9_./-]{1,128}$`)

// ValidatePolicyName enforces the MinIO canned-policy name charset/length.
func ValidatePolicyName(name string) error {
	if !policyNameRE.MatchString(name) {
		return fmt.Errorf("policy name must be 1-128 chars of [A-Za-z0-9_./-]")
	}
	return nil
}

type policyDoc struct {
	Version   string       `json:"Version"`
	Statement []policyStmt `json:"Statement"`
}

type policyStmt struct {
	Effect    string          `json:"Effect"`
	Action    json.RawMessage `json:"Action"`
	NotAction json.RawMessage `json:"NotAction"`
}

var (
	errInvalidJSON      = errors.New("invalid_policy_json")
	errInvalidStructure = errors.New("invalid_policy_structure")
)

// ValidatePolicyDocument performs best-effort structural validation:
//  1. valid JSON object;
//  2. non-empty Statement array, each with Effect in {Allow,Deny} and at
//     least one of Action/NotAction.
//
// Returns errInvalidJSON or errInvalidStructure (sentinels).
func ValidatePolicyDocument(doc []byte) error {
	if !json.Valid(doc) {
		return errInvalidJSON
	}
	var d policyDoc
	if err := json.Unmarshal(doc, &d); err != nil {
		return errInvalidJSON
	}
	if d.Version == "" || len(d.Statement) == 0 {
		return errInvalidStructure
	}
	for _, s := range d.Statement {
		if s.Effect != "Allow" && s.Effect != "Deny" {
			return errInvalidStructure
		}
		if len(s.Action) == 0 && len(s.NotAction) == 0 {
			return errInvalidStructure
		}
	}
	return nil
}

// IsInvalidJSON / IsInvalidStructure let the processor map the sentinels to
// apierror codes without importing apierror here.
func IsInvalidJSON(err error) bool      { return errors.Is(err, errInvalidJSON) }
func IsInvalidStructure(err error) bool { return errors.Is(err, errInvalidStructure) }
