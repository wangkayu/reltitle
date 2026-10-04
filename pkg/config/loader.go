package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadAll reads every config file from baseDir and validates the result.
//
// All files are required. Decoding is strict: an unknown key is an error, so a
// typo cannot silently disable a rule. Every problem found is reported, not just
// the first one.
func LoadAll(baseDir string) (*Config, error) {
	cfg := &Config{}
	files := []struct {
		name string
		out  any
	}{
		{"lexicon.yaml", &cfg.Lexicon},
		{"template_catalog.yaml", &cfg.TemplateCatalog},
		{"inference_rules.yaml", &cfg.Inference},
		{"grouplist.yaml", &cfg.GroupList},
		{"dict_canonical_values.yaml", &cfg.DictCanonicalValues},
		{"dict_token_synonyms.yaml", &cfg.DictTokenSynonyms},
		{"dict_alias_noise_words.yaml", &cfg.DictAliasNoiseWords},
		{"dict_note_promotion.yaml", &cfg.DictNotePromotion},
		{"group_route_overrides.yaml", &cfg.GroupRoute},
		{"route_families.yaml", &cfg.RouteFamilies},
		{"group_slot2_platform_rules.yaml", &cfg.GroupSlot2Platform},
		{"pack_profile_rules.yaml", &cfg.PackProfileRules},
		{"title_knowledge.yaml", &cfg.TitleKnowledge},
	}

	var errs []error
	for _, f := range files {
		if err := loadYAMLFile(filepath.Join(baseDir, f.name), f.out); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", f.name, err))
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("directory %q: %w", baseDir, errors.Join(errs...))
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("directory %q: invalid config: %w", baseDir, err)
	}
	return cfg, nil
}

func loadYAMLFile(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("file is empty")
		}
		return err
	}
	return nil
}
