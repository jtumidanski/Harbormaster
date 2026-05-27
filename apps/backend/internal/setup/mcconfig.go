// Package setup handles the first-run wizard (admin user + MinIO connection
// validation + persist), and the mc-config import helper.
package setup

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
)

// McAlias is a public projection of an mc config alias. Secret keys are
// never embedded here.
type McAlias struct {
	Name          string `json:"name"`
	Endpoint      string `json:"endpoint"`
	AccessKey     string `json:"access_key"`
	TLSSkipVerify bool   `json:"tls_skip_verify"`
}

// ReadMcAliases returns the parsed aliases from the given path. Missing or
// unreadable files return ([], nil). Versions other than "10" return ([], nil)
// with a record of the version encountered in encounteredVersion.
func ReadMcAliases(path string) (aliases []McAlias, encounteredVersion string, err error) {
	data, rerr := os.ReadFile(path)
	if rerr != nil {
		if errors.Is(rerr, fs.ErrNotExist) {
			return nil, "", nil
		}
		return nil, "", nil // best-effort
	}
	var raw struct {
		Version string `json:"version"`
		Aliases map[string]struct {
			URL       string `json:"url"`
			AccessKey string `json:"accessKey"`
			SecretKey string `json:"secretKey"`
			Insecure  bool   `json:"insecure"`
		} `json:"aliases"`
	}
	if uerr := json.Unmarshal(data, &raw); uerr != nil {
		return nil, "", nil
	}
	encounteredVersion = raw.Version
	if raw.Version != "10" {
		return nil, encounteredVersion, nil
	}
	out := make([]McAlias, 0, len(raw.Aliases))
	for name, a := range raw.Aliases {
		out = append(out, McAlias{Name: name, Endpoint: a.URL, AccessKey: a.AccessKey, TLSSkipVerify: a.Insecure})
	}
	return out, encounteredVersion, nil
}

// ReadMcAliasSecret returns the secret key for the named alias, or "" if the
// alias does not exist. Used only at setup-submit time.
func ReadMcAliasSecret(path, alias string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var raw struct {
		Version string `json:"version"`
		Aliases map[string]struct {
			SecretKey string `json:"secretKey"`
		} `json:"aliases"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", err
	}
	if raw.Version != "10" {
		return "", errors.New("unsupported mc config version")
	}
	a, ok := raw.Aliases[alias]
	if !ok {
		return "", errors.New("alias not found")
	}
	return a.SecretKey, nil
}
