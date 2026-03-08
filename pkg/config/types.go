package config

type Config struct {
	Lexicon            LexiconConfig            `json:"lexicon"`
	TemplateCatalog    TemplateCatalogConfig    `json:"template_catalog"`
	Inference          InferenceRulesConfig     `json:"inference"`
	GroupList          GroupListConfig          `json:"group_list"`
	GroupRoute         GroupRouteConfig         `json:"group_route,omitempty"`
	GroupSlot2Platform GroupSlot2PlatformConfig `json:"group_slot2_platform,omitempty"`
	PackProfileRules   PackProfileRulesConfig   `json:"pack_profile_rules,omitempty"`

	DictCanonicalValues     DictCanonicalValuesConfig     `json:"dict_canonical_values,omitempty"`
	DictTokenSynonyms       DictTokenSynonymsConfig       `json:"dict_token_synonyms,omitempty"`
	DictTokenSlotCandidates DictTokenSlotCandidatesConfig `json:"dict_token_slot_candidates,omitempty"`
	DictAliasNoiseWords     DictAliasNoiseWordsConfig     `json:"dict_alias_noise_words,omitempty"`
	DictNotePromotion       DictNotePromotionConfig       `json:"dict_note_promotion,omitempty"`

	AttributeConfig map[string]any `json:"attribute_config,omitempty"`
	BoundaryRules   map[string]any `json:"boundary_rules,omitempty"`
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
	UnknownTemplates   []TemplateDef `yaml:"unknown_templates" json:"unknown_templates"`
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
	Signals      *TemplateSignals    `yaml:"signals" json:"signals"`
}

type TemplateSignals struct {
	SlotOrder               []string `yaml:"slot_order" json:"slot_order"`
	GroupPosition           string   `yaml:"group_position" json:"group_position"`
	GroupAfterEPOnly        bool     `yaml:"group_after_ep_only" json:"group_after_ep_only"`
	ForbidGroupInTitleSpan  bool     `yaml:"forbid_group_in_title_span" json:"forbid_group_in_title_span"`
	EpAnchorPriority        bool     `yaml:"ep_anchor_priority" json:"ep_anchor_priority"`
	TitleOverGroupDefault   bool     `yaml:"title_over_group_default" json:"title_over_group_default"`
	TechStartsAfterEPStrict bool     `yaml:"tech_starts_after_ep" json:"tech_starts_after_ep"`
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
	Version     string         `yaml:"version" json:"version"`
	UpdatedAt   string         `yaml:"last_updated" json:"last_updated"`
	Description string         `yaml:"description" json:"description"`
	Groups      []GroupDef     `yaml:"groups" json:"groups"`
	Statistics  map[string]any `yaml:"statistics,omitempty" json:"statistics,omitempty"`
	UsageNotes  string         `yaml:"usage_notes,omitempty" json:"usage_notes,omitempty"`
}

// GroupDef represents a subtitle group definition
type GroupDef struct {
	Name            string   `yaml:"name" json:"name"`
	Patterns        []string `yaml:"patterns" json:"patterns"`
	Frequency       string   `yaml:"frequency" json:"frequency"`
	ConfidenceBoost float64  `yaml:"confidence_boost" json:"confidence_boost"`
}

type DictCanonicalValuesConfig struct {
	Version     string         `yaml:"version" json:"version"`
	UpdatedAt   string         `yaml:"updated_at" json:"updated_at"`
	Description string         `yaml:"description" json:"description"`
	Canonical   map[string]any `yaml:"canonical" json:"canonical"`
}

type DictTokenSynonymsConfig struct {
	Version     string             `yaml:"version" json:"version"`
	UpdatedAt   string             `yaml:"updated_at" json:"updated_at"`
	Description string             `yaml:"description" json:"description"`
	Rules       []TokenSynonymRule `yaml:"rules" json:"rules"`
}

type TokenSynonymRule struct {
	ID        string             `yaml:"id" json:"id"`
	RawTokens []string           `yaml:"raw_tokens" json:"raw_tokens"`
	Emit      TokenSynonymTarget `yaml:"emit" json:"emit"`
	Priority  int                `yaml:"priority" json:"priority"`
}

type TokenSynonymTarget struct {
	Type                string `yaml:"type" json:"type"`
	Normalized          string `yaml:"normalized" json:"normalized"`
	NormalizedFromToken bool   `yaml:"normalized_from_token" json:"normalized_from_token"`
}

type DictTokenSlotCandidatesConfig struct {
	Version     string               `yaml:"version" json:"version"`
	UpdatedAt   string               `yaml:"updated_at" json:"updated_at"`
	Description string               `yaml:"description" json:"description"`
	Defaults    map[string]any       `yaml:"defaults" json:"defaults"`
	Candidates  []TokenSlotCandidate `yaml:"candidates" json:"candidates"`
}

type TokenSlotCandidate struct {
	Token          string                   `yaml:"token" json:"token"`
	SlotCandidates []TokenSlotCandidateItem `yaml:"slot_candidates" json:"slot_candidates"`
}

type TokenSlotCandidateItem struct {
	Slot       string         `yaml:"slot" json:"slot"`
	Normalized string         `yaml:"normalized" json:"normalized"`
	Priority   int            `yaml:"priority" json:"priority"`
	When       map[string]any `yaml:"when" json:"when"`
}

type DictAliasNoiseWordsConfig struct {
	Version     string           `yaml:"version" json:"version"`
	UpdatedAt   string           `yaml:"updated_at" json:"updated_at"`
	Description string           `yaml:"description" json:"description"`
	AliasNoise  AliasNoiseConfig `yaml:"alias_noise" json:"alias_noise"`
}

type AliasNoiseConfig struct {
	ExactWordsCaseInsensitive       []string `yaml:"exact_words_case_insensitive" json:"exact_words_case_insensitive"`
	RegexPatterns                   []string `yaml:"regex_patterns" json:"regex_patterns"`
	InlineStripWordsCaseInsensitive []string `yaml:"inline_strip_words_case_insensitive" json:"inline_strip_words_case_insensitive"`
}

type DictNotePromotionConfig struct {
	Version       string              `yaml:"version" json:"version"`
	UpdatedAt     string              `yaml:"updated_at" json:"updated_at"`
	Description   string              `yaml:"description" json:"description"`
	Defaults      map[string]any      `yaml:"defaults" json:"defaults"`
	Rules         []NotePromotionRule `yaml:"rules" json:"rules"`
	NoteWhitelist []string            `yaml:"note_whitelist" json:"note_whitelist"`
}

type NotePromotionRule struct {
	ID        string             `yaml:"id" json:"id"`
	Match     NotePromotionMatch `yaml:"match" json:"match"`
	PromoteTo NotePromotionTo    `yaml:"promote_to" json:"promote_to"`
}

type NotePromotionMatch struct {
	NoteValuesCaseInsensitive []string `yaml:"note_values_case_insensitive" json:"note_values_case_insensitive"`
}

type NotePromotionTo struct {
	Type                string `yaml:"type" json:"type"`
	Normalized          string `yaml:"normalized" json:"normalized"`
	NormalizedFromToken bool   `yaml:"normalized_from_token" json:"normalized_from_token"`
}

type GroupRouteConfig struct {
	Version     string               `yaml:"version" json:"version"`
	UpdatedAt   string               `yaml:"updated_at" json:"updated_at"`
	Description string               `yaml:"description" json:"description"`
	Defaults    map[string]any       `yaml:"defaults" json:"defaults"`
	Overrides   []GroupRouteOverride `yaml:"overrides" json:"overrides"`
}

type GroupRouteOverride struct {
	ID       string              `yaml:"id" json:"id"`
	Enabled  bool                `yaml:"enabled" json:"enabled"`
	Priority int                 `yaml:"priority" json:"priority"`
	Match    GroupRouteMatch     `yaml:"match" json:"match"`
	Output   GroupRouteOutput    `yaml:"output" json:"output"`
	Guard    map[string][]string `yaml:"guard" json:"guard"`
}

type GroupRouteMatch struct {
	GroupIn                 []string `yaml:"group_in" json:"group_in"`
	RawMustContainAny       []string `yaml:"raw_must_contain_any" json:"raw_must_contain_any"`
	RequireEpisodeRange     bool     `yaml:"require_episode_range" json:"require_episode_range"`
	RequireEpisodeLike      bool     `yaml:"require_episode_like" json:"require_episode_like"`
	RequireEpisodeWithTitle bool     `yaml:"require_episode_with_title" json:"require_episode_with_title"`
}

type GroupRouteOutput struct {
	ForceGroup         string   `yaml:"force_group" json:"force_group"`
	ForceTitle         string   `yaml:"force_title" json:"force_title"`
	ClearAliases       bool     `yaml:"clear_aliases" json:"clear_aliases"`
	ForceTemplateID    string   `yaml:"force_template_id" json:"force_template_id"`
	ForceContentKind   string   `yaml:"force_content_kind" json:"force_content_kind"`
	ForceRouteID       string   `yaml:"force_route_id" json:"force_route_id"`
	TitleStripPrefixes []string `yaml:"title_strip_prefixes" json:"title_strip_prefixes"`
	AliasStripPrefixes []string `yaml:"alias_strip_prefixes" json:"alias_strip_prefixes"`
}

type GroupSlot2PlatformConfig struct {
	Version     string                         `yaml:"version" json:"version"`
	UpdatedAt   string                         `yaml:"updated_at" json:"updated_at"`
	Description string                         `yaml:"description" json:"description"`
	Rules       []GroupSlot2PlatformRuleConfig `yaml:"rules" json:"rules"`
}

type GroupSlot2PlatformRuleConfig struct {
	ID        string                              `yaml:"id" json:"id"`
	Enabled   bool                                `yaml:"enabled" json:"enabled"`
	Priority  int                                 `yaml:"priority" json:"priority"`
	Match     GroupSlot2PlatformMatchConfig       `yaml:"match" json:"match"`
	Keywords  GroupSlot2PlatformKeywordsConfig    `yaml:"keywords" json:"keywords"`
	Whitelist []GroupSlot2PlatformWhitelistConfig `yaml:"platform_whitelist" json:"platform_whitelist"`
	Output    GroupSlot2PlatformOutputConfig      `yaml:"output" json:"output"`
}

type GroupSlot2PlatformMatchConfig struct {
	GroupIn    []string `yaml:"group_in" json:"group_in"`
	GroupRegex string   `yaml:"group_regex" json:"group_regex"`
	SlotIndex  int      `yaml:"slot_index" json:"slot_index"`
}

type GroupSlot2PlatformKeywordsConfig struct {
	Yue          []string `yaml:"yue" json:"yue"`
	Distributor  []string `yaml:"distributor" json:"distributor"`
	LanguageMeta []string `yaml:"language_meta" json:"language_meta"`
	TrimForInfer []string `yaml:"trim_for_infer" json:"trim_for_infer"`
}

type GroupSlot2PlatformWhitelistConfig struct {
	ContainsAny []string `yaml:"contains_any" json:"contains_any"`
	Normalized  string   `yaml:"normalized" json:"normalized"`
}

type GroupSlot2PlatformOutputConfig struct {
	EmitAudioLang             string  `yaml:"emit_audio_lang" json:"emit_audio_lang"`
	NoteOnDistributor         string  `yaml:"note_on_distributor" json:"note_on_distributor"`
	InferUnknownPlatform      bool    `yaml:"infer_unknown_platform" json:"infer_unknown_platform"`
	UnknownPlatformConfidence float64 `yaml:"unknown_platform_confidence" json:"unknown_platform_confidence"`
}

type PackProfileRulesConfig struct {
	UpdatedAt   string                  `yaml:"updated_at" json:"updated_at"`
	Description string                  `yaml:"description" json:"description"`
	Rules       []PackProfileRuleConfig `yaml:"rules" json:"rules"`
}

type PackProfileRuleConfig struct {
	ID       string                `yaml:"id" json:"id"`
	Enabled  bool                  `yaml:"enabled" json:"enabled"`
	Priority int                   `yaml:"priority" json:"priority"`
	When     PackProfileWhenConfig `yaml:"when" json:"when"`
	Emit     string                `yaml:"emit" json:"emit"`
}

type PackProfileWhenConfig struct {
	ContentKindIn       []string `yaml:"content_kind_in" json:"content_kind_in"`
	PackKindIn          []string `yaml:"pack_kind_in" json:"pack_kind_in"`
	PackKindNotIn       []string `yaml:"pack_kind_not_in" json:"pack_kind_not_in"`
	RequiresContentsAll []string `yaml:"requires_contents_all" json:"requires_contents_all"`
	RequiresContentsAny []string `yaml:"requires_contents_any" json:"requires_contents_any"`
	ForbidContentsAny   []string `yaml:"forbid_contents_any" json:"forbid_contents_any"`
	HasSeason           *bool    `yaml:"has_season" json:"has_season"`
	HasSeasonRange      *bool    `yaml:"has_season_range" json:"has_season_range"`
	MinContentsDistinct int      `yaml:"min_contents_distinct" json:"min_contents_distinct"`
}
