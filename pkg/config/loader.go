package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func LoadAll(baseDir string) (*Config, error) {
	cfg := &Config{}

	lexiconPath := filepath.Join(baseDir, "lexicon_v1.yaml")
	if err := loadYAMLFile(lexiconPath, &cfg.Lexicon); err != nil {
		return nil, fmt.Errorf("load lexicon: %w", err)
	}

	templateCatalogPath := filepath.Join(baseDir, "template_catalog.yaml")
	if err := loadYAMLFile(templateCatalogPath, &cfg.TemplateCatalog); err != nil {
		return nil, fmt.Errorf("load template_catalog: %w", err)
	}

	inferenceRulesPath := filepath.Join(baseDir, "inference_rules.yaml")
	if err := loadYAMLFile(inferenceRulesPath, &cfg.Inference); err != nil {
		return nil, fmt.Errorf("load inference_rules: %w", err)
	}

	attributeConfigPath := filepath.Join(baseDir, "attribute_config.yaml")
	_ = loadYAMLFile(attributeConfigPath, &cfg.AttributeConfig)

	boundaryRulesPath := filepath.Join(baseDir, "boundary_rules.yaml")
	_ = loadYAMLFile(boundaryRulesPath, &cfg.BoundaryRules)

	// Load group list (optional, for token identification assistance)
	groupListPath := filepath.Join(baseDir, "grouplist.yaml")
	if err := loadYAMLFile(groupListPath, &cfg.GroupList); err != nil {
		// GroupList is optional, log but don't fail
		fmt.Printf("Warning: failed to load grouplist.yaml: %v\n", err)
	}

	_ = loadYAMLFile(filepath.Join(baseDir, "dict_canonical_values_v25.yaml"), &cfg.DictCanonicalValues)
	_ = loadYAMLFile(filepath.Join(baseDir, "dict_token_synonyms_v25.yaml"), &cfg.DictTokenSynonyms)
	_ = loadYAMLFile(filepath.Join(baseDir, "dict_token_slot_candidates_v25.yaml"), &cfg.DictTokenSlotCandidates)
	_ = loadYAMLFile(filepath.Join(baseDir, "dict_alias_noise_words_v25.yaml"), &cfg.DictAliasNoiseWords)
	_ = loadYAMLFile(filepath.Join(baseDir, "dict_note_promotion_v25.yaml"), &cfg.DictNotePromotion)
	_ = loadYAMLFile(filepath.Join(baseDir, "group_route_overrides_v25.yaml"), &cfg.GroupRoute)

	return cfg, nil
}

func loadYAMLFile(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, out)
}
