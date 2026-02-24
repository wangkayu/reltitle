package config

type Config struct {
	Lexicon         LexiconConfig         `json:"lexicon"`
	TemplateCatalog TemplateCatalogConfig `json:"template_catalog"`
	Inference       InferenceRulesConfig  `json:"inference"`
	GroupList       GroupListConfig       `json:"group_list"`

	AttributeConfig    map[string]any `json:"attribute_config,omitempty"`
	BoundaryRules      map[string]any `json:"boundary_rules,omitempty"`
	// TailgroupWhitelist removed - now using GroupList only
}

type LexiconConfig struct {
	Version   string         `yaml:"version" json:"version"`
	UpdatedAt string         `yaml:"updated_at" json:"updated_at"`
	Entries   []LexiconEntry `yaml:"entries" json:"entries"`
}

type LexiconEntry struct {
	ID         string          `yaml:"id" json:"id"`
	Scope      string          `yaml:"scope" json:"scope"`
	Tokens     []string        `yaml:"tokens" json:"tokens"`
	AnyOf      []LexiconClause `yaml:"any_of" json:"any_of"`
	Emit       []LexiconEmit   `yaml:"emit" json:"emit"`
	Remove     bool            `yaml:"remove" json:"remove"`
	Confidence float64         `yaml:"confidence" json:"confidence"`
}

type LexiconClause struct {
	Scope      string        `yaml:"scope" json:"scope"`
	Tokens     []string      `yaml:"tokens" json:"tokens"`
	Emit       []LexiconEmit `yaml:"emit" json:"emit"`
	Remove     bool          `yaml:"remove" json:"remove"`
	Confidence float64       `yaml:"confidence" json:"confidence"`
}

type LexiconEmit struct {
	Type       string `yaml:"type" json:"type"`
	Normalized string `yaml:"normalized" json:"normalized"`
	TokenIndex int    `yaml:"token_index" json:"token_index"`
}

type InferenceRulesConfig struct {
	Version              string                     `yaml:"version" json:"version"`
	UpdatedAt            string                     `yaml:"updated_at" json:"updated_at"`
	Description          string                     `yaml:"description" json:"description"`
	ContentTypeInference []ContentTypeInferenceRule `yaml:"content_type_inference" json:"content_type_inference"`
}

type ContentTypeInferenceRule struct {
	Name         string         `yaml:"name" json:"name"`
	Priority     int            `yaml:"priority" json:"priority"`
	Condition    map[string]any `yaml:"condition" json:"condition"`
	Result       string         `yaml:"result" json:"result"`
	Confidence   float64        `yaml:"confidence" json:"confidence"`
	Description  string         `yaml:"description" json:"description"`
	Keywords     []string       `yaml:"keywords" json:"keywords"`
	PackKeywords []string       `yaml:"pack_keywords" json:"pack_keywords"`
}

type TemplateCatalogConfig struct {
	Version            string        `yaml:"version" json:"version"`
	UpdatedAt          string        `yaml:"updated_at" json:"updated_at"`
	Description        string        `yaml:"description" json:"description"`
	TVEpisodeTemplates []TemplateDef `yaml:"tv_episode_templates" json:"tv_episode_templates"`
	BatchTemplates     []TemplateDef `yaml:"batch_templates" json:"batch_templates"`
	MovieTemplates     []TemplateDef `yaml:"movie_templates" json:"movie_templates"`
	SpecialTemplates   []TemplateDef `yaml:"special_templates" json:"special_templates"`
}

type TemplateDef struct {
	ID           string              `yaml:"id" json:"id"`
	Name         string              `yaml:"name" json:"name"`
	ContentTypes []string            `yaml:"content_types" json:"content_types"`
	Signature    string              `yaml:"signature" json:"signature"`
	Confidence   float64             `yaml:"confidence" json:"confidence"`
	Description  string              `yaml:"description" json:"description"`
	Examples     []string            `yaml:"examples" json:"examples"`
	RouteID      string              `yaml:"route_id" json:"route_id"`
	Conditions   *TemplateConditions `yaml:"conditions" json:"conditions"`
}

type TemplateConditions struct {
	Require *TemplateConditionRequire `yaml:"require" json:"require"`
}

type TemplateConditionRequire struct {
	PlatformIn   []string `yaml:"platform_in" json:"platform_in"`
	SourceTypeIn []string `yaml:"source_type_in" json:"source_type_in"`
	HasSxxExx    bool     `yaml:"has_sxxexx" json:"has_sxxexx"`
	HasTailGroup bool     `yaml:"has_tail_group" json:"has_tail_group"`
}

// GroupListConfig represents the group list configuration
type GroupListConfig struct {
	Version     string     `yaml:"version" json:"version"`
	UpdatedAt   string     `yaml:"last_updated" json:"last_updated"`
	Description string     `yaml:"description" json:"description"`
	Groups      []GroupDef `yaml:"groups" json:"groups"`
	Statistics  map[string]any `yaml:"statistics,omitempty" json:"statistics,omitempty"`
	UsageNotes  string     `yaml:"usage_notes,omitempty" json:"usage_notes,omitempty"`
}

// GroupDef represents a subtitle group definition
type GroupDef struct {
	Name            string   `yaml:"name" json:"name"`
	Patterns        []string `yaml:"patterns" json:"patterns"`
	Frequency       string   `yaml:"frequency" json:"frequency"`
	ConfidenceBoost float64  `yaml:"confidence_boost" json:"confidence_boost"`
}
