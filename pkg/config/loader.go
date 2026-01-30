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

	tailgroupWhitelistPath := filepath.Join(baseDir, "tailgroup_whitelist.yaml")
	_ = loadYAMLFile(tailgroupWhitelistPath, &cfg.TailgroupWhitelist)

	return cfg, nil
}

func loadYAMLFile(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, out)
}
