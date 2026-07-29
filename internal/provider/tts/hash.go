package tts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// MetaHash computes a deterministic SHA-256 hash of the ProviderMeta content.
// Any change to Name, Description, DefaultBaseURL, Models (including their
// VoiceInfo lists), or Features will produce a different hash.
func (m ProviderMeta) MetaHash() string {
	// Build a deterministic representation: sorted model keys, sorted voices.
	type hashVoice struct {
		VoiceID     string
		Name        string
		Gender      string
		Description string
		Languages   []string
		Tags        []string
		SampleURL   string
		Emotions    []string
	}

	type hashModel struct {
		Name               string
		SupportedLanguages []string
		Voices             []hashVoice
	}

	type hashInput struct {
		Name           string
		Description    string
		DefaultBaseURL string
		Models         []hashModel
		Features       []Feature
	}

	input := hashInput{
		Name:           m.Name,
		Description:    m.Description,
		DefaultBaseURL: m.DefaultBaseURL,
		Features:       sortedFeatures(m.Features),
	}

	// Sort model names for determinism.
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

		for _, v := range info.SystemVoices {
			hv := hashVoice{
				VoiceID:     v.VoiceID,
				Name:        v.Name,
				Gender:      v.Gender,
				Description: v.Description,
				SampleURL:   v.SampleURL,
			}
			for _, l := range v.Languages {
				hv.Languages = append(hv.Languages, string(l))
			}
			sort.Strings(hv.Languages)
			hv.Tags = sortedStrings(v.Tags)
			hv.Emotions = sortedStrings(v.Emotions)
			hm.Voices = append(hm.Voices, hv)
		}
		// Sort voices by VoiceID for determinism.
		sort.Slice(hm.Voices, func(i, j int) bool { return hm.Voices[i].VoiceID < hm.Voices[j].VoiceID })
		input.Models = append(input.Models, hm)
	}

	b, err := json.Marshal(input)
	if err != nil {
		return "error"
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func sortedStrings(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	out := make([]string, len(s))
	copy(out, s)
	sort.Strings(out)
	return out
}

func sortedFeatures(f []Feature) []Feature {
	if len(f) == 0 {
		return nil
	}
	out := make([]Feature, len(f))
	copy(out, f)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// MetaHash computes a deterministic SHA-256 hash of a single VoiceInfo.
func (v VoiceInfo) MetaHash() string {
	type hashVoice struct {
		VoiceID     string
		Name        string
		Gender      string
		Description string
		Languages   []string
		Tags        []string
		SampleURL   string
		Emotions    []string
	}

	hv := hashVoice{
		VoiceID:     v.VoiceID,
		Name:        v.Name,
		Gender:      v.Gender,
		Description: v.Description,
		SampleURL:   v.SampleURL,
	}
	for _, l := range v.Languages {
		hv.Languages = append(hv.Languages, string(l))
	}
	sort.Strings(hv.Languages)
	hv.Tags = sortedStrings(v.Tags)
	hv.Emotions = sortedStrings(v.Emotions)

	b, err := json.Marshal(hv)
	if err != nil {
		return "error"
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
