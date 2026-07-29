package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// MetaHash computes a deterministic SHA-256 hash of the ProviderMeta content.
func (m ProviderMeta) MetaHash() string {
	type hashModel struct {
		Name               string
		SupportedLanguages []string
	}

	type hashInput struct {
		Name           string
		DefaultBaseURL string
		Models         []hashModel
	}

	input := hashInput{
		Name:           m.Name,
		DefaultBaseURL: m.DefaultBaseURL,
	}

	modelNames := make([]string, 0, len(m.Models))
	for name := range m.Models {
		modelNames = append(modelNames, name)
	}
	sort.Strings(modelNames)

	for _, name := range modelNames {
		info := m.Models[name]
		hm := hashModel{Name: name}
		for _, lang := range info.SupportedLanguages {
			hm.SupportedLanguages = append(hm.SupportedLanguages, string(lang))
		}
		sort.Strings(hm.SupportedLanguages)
		input.Models = append(input.Models, hm)
	}

	b, err := json.Marshal(input)
	if err != nil {
		return "error"
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
