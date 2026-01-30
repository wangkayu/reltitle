# 配置驱动Parser重构 - 设计文档

**Feature Name**: config-driven-parser-refactor  
**Created**: 2025-01-17  
**Status**: Draft  
**Priority**: P0

---

## 1. 设计原则

### 1.1 现实认知

**核心认知**: 配置驱动不是银弹,不能解决所有问题

**配置驱动能做什么**:
- ✅ 组织硬编码,让它更清晰
- ✅ 规范规则添加流程
- ✅ 让"补"机制更可控
- ✅ 降低维护成本

**配置驱动不能做什么**:
- ❌ 消除所有硬编码
- ❌ 自动补充缺失的规则
- ❌ 消除"补"机制的复杂性
- ❌ 自动提高识别准确率

### 1.2 设计目标

**短期目标** (Phase 1 - 本周):
- 配置化现有逻辑,功能不变
- 降低维护成本
- 规范规则添加流程

**中期目标** (Phase 2 - 1-2周):
- 补充Lexicon规则 (覆盖率 30% → 50%)
- 减少"补"机制依赖
- 提高字段独立性

**长期目标** (Phase 3 - 1-2月):
- 实现规则引擎
- 配置化"补"规则
- 深度改进识别机制

---

## 2. 当前识别机制分析

### 2.1 识别流程的真相

```
Raw Title → Lexicon匹配 (30%) → 边界识别 (硬编码) → 
  → TITLE提取 ("补"机制) → 模板匹配 → 基于模板调整 ("补"机制)
```

**关键发现**:
1. **Lexicon只识别30%** - GROUP, EP, TECH等基础属性
2. **边界识别是硬编码的** - 强边界、弱边界、修饰符
3. **TITLE是"补"出来的** - 位置推理 + 清理
4. **Aliases是"补"出来的** - 从分隔符拆分
5. **70%依赖"补"机制** - 无法完全消除

### 2.2 Lexicon覆盖率分析

| 类别 | 当前数量 | 需要数量 | 覆盖率 |
|------|---------|---------|--------|
| 技术属性 | ~50 | ~100 | 50% |
| 语言标记 | ~30 | ~50 | 60% |
| 平台标记 | ~10 | ~30 | 33% |
| 地区标记 | 0 | ~20 | 0% |
| 内容分级 | 5 | ~15 | 33% |
| 版本标记 | ~10 | ~20 | 50% |
| **总计** | **~110** | **~250** | **44%** |

**结论**: 需要补充约140个Lexicon规则

### 2.3 "补"机制分析

**当前的"补"函数** (硬编码):
1. `applyGroupSpecificEpisodeRules` - Group特定规则
2. `applyEpisodeTitleBetweenEpAndTech` - 位置推理
3. `fillMissingTitleForGroupEpSubFamily` - 模板家族规则
4. `addTitleAliasesFromBracketTitleBlocks` - 括号Aliases
5. `addPipeSeparatedTailAliases` - TAIL Aliases
6. `refineTitleSlotsByFamily` - 基于模板的Title调整
7. `normalizeTitleAttrsForEmbedding` - Title标准化
8. `dropTechyTitleAliases` - 过滤tech noise
9. `ensureTitleAnchors` - 确保Title锚点
10. `detectTailGroup` - TAILGROUP检测

**问题**:
- 每个函数都是针对特定case的硬编码逻辑
- 没有统一的"补"机制
- 添加新的"补"逻辑需要写新函数

**结论**: "补"机制是必需的,但需要更清晰的组织

---

## 3. 架构设计

### 3.1 分层架构 (改进版)

```
Layer 1: Tokenization (分词)
Layer 2: Lexicon Matching (字典匹配) ← 配置驱动
Layer 3: Signal Detection (信号识别) ← 新增层
  ├── Movie信号 (movie_index, sp_kind=MOVIE)
  ├── Episode信号 (episode单集)
  ├── Pack信号 (pack_kind, episode_range, season无episode)
  └── Special信号 (sp_kind, sp_index)
Layer 4: Content Type Inference (类型推理) ← 新增层,在模板匹配之前
  └── Movie > TV_EP > Pack > SP
Layer 5: Template Matching (模板匹配) ← 基于Content Type过滤模板
Layer 6: Field Extraction (字段提取) ← 配置驱动
Layer 7: Inference (推理) ← 配置驱动
Layer 8: Cleaning (清理) ← 配置驱动
Layer 9: Output Building (输出构建)
```

**关键改进**:
1. **Content Type在模板匹配之前确定** - 避免被带入错误的模板
2. **模板匹配基于Content Type过滤** - 只考虑适用的模板
3. **每一层都基于配置文件工作** - 但"补"机制仍然存在

### 3.2 配置文件架构

```
training/resources/
├── attribute_config.yaml       # 字段定义和业务规则
├── lexicon_v1.yaml            # Lexicon字典 (现有)
├── boundary_rules.yaml        # 边界规则 (新增)
├── inference_rules.yaml       # 推理规则 (新增)
└── tailgroup_whitelist.yaml   # TAILGROUP白名单 (现有)
```

#### 3.2.1 attribute_config.yaml (已创建)

**内容**:
- 字段定义 (name, type, required, business_priority)
- 提取规则 (source, method, post_process)
- 验证规则 (pattern, range, length)
- 标准化规则 (mapping, format, trim)
- 业务规则 (rss_matching, quality_check)
- 字段依赖关系 (independent_fields, derived_fields)

**状态**: ✅ 已创建

#### 3.2.2 boundary_rules.yaml (新增)

**目的**: 将边界识别规则从代码移到配置文件

**内容**:
```yaml
version: "1.0.0"

# 强边界定义
strong_boundaries:
  - type: episode
    reason: "EP是最强边界,前后内容不应混入TITLE"
    priority: 100
  
  - type: tech
    reason: "TECH是强边界,前后内容不应混入TITLE"
    priority: 90
    subtypes:
      - resolution
      - video_codec
      - audio_codec
      - source_type
  
  - type: platform
    reason: "Platform是强边界"
    priority: 80

# 弱边界定义
weak_boundaries:
  - type: note
    reason: "Note是弱边界,可能混入TITLE"
    priority: 30
  
  - type: noise
    reason: "Noise是弱边界,应该过滤"
    priority: 10

# 标题修饰符
title_modifiers:
  japanese:
    - pattern: "篇"
      reason: "日文篇章标记"
    - pattern: "編"
      reason: "日文编章标记"
    - pattern: "章"
      reason: "日文章节标记"
  
  english:
    - pattern: "season"
      case_insensitive: true
      reason: "季度标记"
    - pattern: "part"
      case_insensitive: true
      reason: "部分标记"
    - pattern: "final"
      case_insensitive: true
      reason: "最终标记"
    - pattern: "arc"
      case_insensitive: true
      reason: "故事弧标记"
    - pattern: "saga"
      case_insensitive: true
      reason: "传奇标记"

# 边界检测规则
boundary_detection:
  title_extraction:
    - rule: "between_group_and_episode"
      condition:
        - has_group: true
        - has_episode: true
      extract_from:
        start: after_group
        end: before_episode
      confidence: 0.9
    
    - rule: "between_group_and_tech"
      condition:
        - has_group: true
        - has_tech: true
        - no_episode: true
      extract_from:
        start: after_group
        end: before_tech
      confidence: 0.7
```

**状态**: ⏳ 待创建

#### 3.2.3 inference_rules.yaml (新增)

**目的**: 将推理规则从代码移到配置文件

**内容**:
```yaml
version: "1.0.0"

# Content Kind推理规则
content_kind_inference:
  - name: "tv_episode"
    condition:
      - has_episode: true
      - no_episode_range: true
      - no_movie_keyword: true
    result: "TV_EP"
    confidence: 0.9
  
  - name: "batch"
    condition:
      - has_episode_range: true
    result: "BATCH"
    confidence: 0.95
  
  - name: "movie"
    condition:
      - has_movie_keyword: true
    result: "MOVIE"
    confidence: 0.85
    keywords:
      - "movie"
      - "劇場版"
      - "剧场版"
      - "电影"
  
  - name: "ova"
    condition:
      - has_ova_keyword: true
    result: "OVA"
    confidence: 0.85
    keywords:
      - "ova"
      - "oav"

# Episode推理规则
episode_inference:
  - name: "explicit_pattern"
    pattern: "E\\d{2,4}"
    confidence: 0.95
    priority: 100
  
  - name: "positional_after_title"
    condition:
      - has_title: true
      - has_tech: true
    extract_from:
      start: after_title
      end: before_tech
    pattern: "\\d{1,4}"
    confidence: 0.8
    priority: 80
  
  - name: "template_based"
    condition:
      - has_template: true
      - template_has_ep_slot: true
    extract_from:
      slot: "EP"
    confidence: 0.7
    priority: 60

# Title推理规则
title_inference:
  - name: "between_group_and_episode"
    condition:
      - has_group: true
      - has_episode: true
      - no_title: true
    extract_from:
      start: after_group
      end: before_episode
    post_process:
      - trim_delimiters: " /|｜\\"
      - remove_tech_noise: true
      - standardize_spaces: true
    confidence: 0.9
  
  - name: "from_plain_segment"
    condition:
      - has_plain_segment: true
      - no_title: true
    extract_from:
      segment_type: "plain"
      filter_by_length: true
      min_length: 2
    confidence: 0.7

# Aliases推理规则
aliases_inference:
  - name: "multi_language_split"
    condition:
      - has_title: true
      - title_contains_delimiter: true
    split_by: ["/", "|", "｜"]
    deduplicate: true
    exclude_main_title: true
    confidence: 0.8
  
  - name: "from_container"
    condition:
      - has_container_segment: true
    extract_from:
      segment_type: "container"
      filter_tech: true
    confidence: 0.7
  
  - name: "from_tail"
    condition:
      - has_tail_segment: true
    extract_from:
      segment_type: "tail"
      split_by: ["|", "｜"]
    confidence: 0.6
```

**状态**: ⏳ 待创建

---

## 4. 核心组件设计

### 4.1 配置加载器 (Config Loader)

**职责**: 加载所有配置文件,提供统一的查询接口

**接口**:
```go
type ConfigLoader interface {
    // 加载所有配置
    LoadAll() error
    
    // 查询字段配置
    GetFieldConfig(fieldName string) (*FieldConfig, error)
    
    // 查询边界规则
    GetBoundaryRules() (*BoundaryRules, error)
    
    // 查询推理规则
    GetInferenceRules(ruleType string) ([]InferenceRule, error)
    
    // 查询业务规则
    GetBusinessRule(ruleName string) (*BusinessRule, error)
}

type Config struct {
    Attributes  *AttributeConfig
    Boundaries  *BoundaryRules
    Inference   *InferenceRules
    Lexicon     *LexiconConfig
    Tailgroup   *TailgroupWhitelist
}
```

**实现**:
```go
type FileConfigLoader struct {
    basePath string
    config   *Config
    once     sync.Once
}

func (l *FileConfigLoader) LoadAll() error {
    l.once.Do(func() {
        // 加载所有配置文件
        l.config = &Config{}
        l.config.Attributes = loadAttributeConfig(l.basePath + "/attribute_config.yaml")
        l.config.Boundaries = loadBoundaryRules(l.basePath + "/boundary_rules.yaml")
        l.config.Inference = loadInferenceRules(l.basePath + "/inference_rules.yaml")
        l.config.Lexicon = loadLexiconConfig(l.basePath + "/lexicon_v1.yaml")
        l.config.Tailgroup = loadTailgroupWhitelist(l.basePath + "/tailgroup_whitelist.yaml")
    })
    return nil
}
```

**特点**:
- 使用`sync.Once`确保只加载一次
- 提供统一的查询接口
- 支持热重载 (未来)

### 4.2 边界识别器 (Boundary Detector)

**职责**: 基于配置文件识别边界

**接口**:
```go
type BoundaryDetector interface {
    // 判断属性是否是强边界
    IsStrongBoundary(attrType model.AttrType) bool
    
    // 判断token是否是标题修饰符
    IsTitleModifier(tokenText string) bool
    
    // 检查token是否被强边界包围
    IsTokenSurroundedByStrongBoundaries(tokenIdx int, seg model.Segment, attrs []model.Attribute) bool
    
    // 根据边界规则过滤标题tokens
    FilterTitleTokensByBoundaries(tokens []model.Token, seg model.Segment, attrs []model.Attribute) []model.Token
}
```

**实现**:
```go
type ConfigBoundaryDetector struct {
    config *BoundaryRules
}

func (d *ConfigBoundaryDetector) IsStrongBoundary(attrType model.AttrType) bool {
    // 从配置文件查询
    for _, boundary := range d.config.StrongBoundaries {
        if boundary.Type == attrType {
            return true
        }
        // 检查subtypes
        for _, subtype := range boundary.Subtypes {
            if subtype == attrType {
                return true
            }
        }
    }
    return false
}

func (d *ConfigBoundaryDetector) IsTitleModifier(tokenText string) bool {
    // 从配置文件查询
    text := strings.TrimSpace(tokenText)
    lowerText := strings.ToLower(text)
    
    // 检查日文修饰符
    for _, mod := range d.config.TitleModifiers.Japanese {
        if strings.Contains(text, mod.Pattern) {
            return true
        }
    }
    
    // 检查英文修饰符
    for _, mod := range d.config.TitleModifiers.English {
        pattern := mod.Pattern
        if mod.CaseInsensitive {
            if strings.Contains(lowerText, strings.ToLower(pattern)) {
                return true
            }
        } else {
            if strings.Contains(text, pattern) {
                return true
            }
        }
    }
    
    return false
}
```

**特点**:
- 完全基于配置文件
- 不需要修改代码就能添加新规则
- 保持与现有代码的兼容性

### 4.3 推理引擎 (Inference Engine)

**职责**: 基于配置文件的推理规则推理缺失字段

**接口**:
```go
type InferenceEngine interface {
    // 推理Content Kind
    InferContentKind(ctx InferenceContext) (string, float64, error)
    
    // 推理Episode
    InferEpisode(ctx InferenceContext) (string, float64, error)
    
    // 推理Title
    InferTitle(ctx InferenceContext) (string, float64, error)
    
    // 推理Aliases
    InferAliases(ctx InferenceContext) ([]string, float64, error)
}

type InferenceContext struct {
    RawTitle  string
    Segments  []model.Segment
    Attrs     []model.Attribute
    Fields    map[string]interface{}
    Template  *Template
    Config    *InferenceRules
}
```

**实现**:
```go
type ConfigInferenceEngine struct {
    config *InferenceRules
}

func (e *ConfigInferenceEngine) InferContentKind(ctx InferenceContext) (string, float64, error) {
    // 遍历推理规则
    for _, rule := range e.config.ContentKindInference {
        // 检查条件
        if e.checkCondition(rule.Condition, ctx) {
            return rule.Result, rule.Confidence, nil
        }
    }
    
    // 默认
    return "UNKNOWN", 0.5, nil
}

func (e *ConfigInferenceEngine) checkCondition(conditions []Condition, ctx InferenceContext) bool {
    for _, cond := range conditions {
        switch cond.Type {
        case "has_episode":
            if !hasField(ctx.Fields, "episode") {
                return false
            }
        case "no_episode_range":
            if hasField(ctx.Fields, "episode_range") {
                return false
            }
        case "has_movie_keyword":
            if !containsKeyword(ctx.Fields["title"], cond.Keywords) {
                return false
            }
        // ... 其他条件
        }
    }
    return true
}
```

**特点**:
- 完全基于配置文件的规则
- 支持复杂的条件判断
- 易于添加新的推理规则

### 4.4 字段提取器 (Field Extractor)

**职责**: 基于配置文件提取字段

**接口**:
```go
type FieldExtractor interface {
    Extract(ctx ExtractionContext) (interface{}, error)
}

type ExtractionContext struct {
    RawTitle  string
    Segments  []model.Segment
    Attrs     []model.Attribute
    Slots     map[string]Slot
    Config    *FieldConfig
}
```

**实现示例** (TitleExtractor):
```go
type TitleExtractor struct {
    config          *FieldConfig
    boundaryDetector *BoundaryDetector
    inferenceEngine  *InferenceEngine
}

func (e *TitleExtractor) Extract(ctx ExtractionContext) (interface{}, error) {
    // 1. 尝试从TITLE slot提取
    if titleSlot, ok := ctx.Slots["TITLE"]; ok {
        title := titleSlot.Text
        
        // 2. 应用post_process规则 (从配置文件)
        for _, process := range e.config.Extraction.PostProcess {
            switch process.Type {
            case "trim_delimiters":
                title = trimDelimiters(title, process.Delimiters)
            case "remove_tech_noise":
                title = removeTechNoise(title)
            case "standardize_spaces":
                title = standardizeSpaces(title)
            }
        }
        
        // 3. 应用验证规则 (从配置文件)
        if err := e.validate(title); err != nil {
            return nil, err
        }
        
        // 4. 应用标准化规则 (从配置文件)
        title = e.standardize(title)
        
        return title, nil
    }
    
    // 5. 如果没有TITLE slot,使用推理引擎
    if e.config.Extraction.Fallback == "inference" {
        title, _, err := e.inferenceEngine.InferTitle(ctx)
        return title, err
    }
    
    return nil, errors.New("no title found")
}
```

**特点**:
- 基于配置文件的提取规则
- 支持post_process、validation、standardization
- 支持fallback到推理引擎

---

## 5. "补"机制的组织

### 5.1 "补"机制的分类

**Type 1: 位置推理** (可配置化)
- 从GROUP和EP之间提取TITLE
- 从EP和TECH之间提取TITLE
- 从TAIL中提取Aliases

**配置方式**:
```yaml
# boundary_rules.yaml
boundary_detection:
  title_extraction:
    - rule: "between_group_and_episode"
      extract_from:
        start: after_group
        end: before_episode
```

**Type 2: 模板家族规则** (可配置化)
- 基于模板家族调整字段
- 基于模板家族分类Aliases

**配置方式**:
```yaml
# inference_rules.yaml
template_family_rules:
  TF02:  # [GROUP] TITLE - EP [TECH]
    title_extraction:
      position: between_group_and_episode
    aliases_classification:
      head: []
      mid: from_multi_language_split
      tail: []
```

**Type 3: Group特定规则** (部分可配置化)
- 针对特定Group的EP规则
- 针对特定Group的Title规则

**配置方式**:
```yaml
# inference_rules.yaml
group_specific_rules:
  LoliHouse:
    episode_extraction:
      - pattern: "\\d{2}"
        position: after_title
    title_extraction:
      - position: after_group
        before: episode
```

**Type 4: 复杂逻辑** (难以配置化)
- 跨Slot的复杂推理
- 需要多次迭代的逻辑
- 需要状态管理的逻辑

**处理方式**: 保留代码实现,但通过配置控制触发条件

### 5.2 "补"机制的执行顺序

```
Phase 1: Lexicon匹配 (30%覆盖率)
  ↓
Phase 2: 位置推理 ("补"机制 Type 1) - 配置驱动
  ↓
Phase 3: 模板匹配
  ↓
Phase 4: 模板家族规则 ("补"机制 Type 2) - 配置驱动
  ↓
Phase 5: Group特定规则 ("补"机制 Type 3) - 部分配置驱动
  ↓
Phase 6: 复杂逻辑 ("补"机制 Type 4) - 代码实现
```

**关键**: 大部分"补"机制可以配置化,但复杂逻辑仍需代码实现

---

## 6. 实施计划

### Phase 1: 配置化现有逻辑 (本周)

**目标**: 将现有的硬编码移到配置文件,功能不变

**任务**:
1. ✅ 创建attribute_config.yaml (已完成)
2. ⏳ 创建boundary_rules.yaml
3. ⏳ 创建inference_rules.yaml
4. ⏳ 实现ConfigLoader
5. ⏳ 实现ConfigBoundaryDetector
6. ⏳ 重构slot_boundary.go使用ConfigBoundaryDetector
7. ⏳ 测试功能不变

**验收标准**:
- [ ] 所有配置文件创建完成
- [ ] ConfigLoader可以加载所有配置
- [ ] 边界识别使用配置文件
- [ ] 功能测试通过 (与旧版本输出一致)

### Phase 2: 补充Lexicon规则 (1-2周)

**目标**: 提高Lexicon覆盖率 (30% → 50%)

**任务**:
1. ⏳ 补充平台标记 (~20个)
2. ⏳ 补充地区标记 (~20个)
3. ⏳ 补充内容分级 (~10个)
4. ⏳ 补充版本标记 (~10个)
5. ⏳ 补充完结标记 (~10个)
6. ⏳ 测试覆盖率提升

**验收标准**:
- [ ] Lexicon规则数量 > 180
- [ ] Lexicon覆盖率 > 50%
- [ ] 新规则测试通过

### Phase 3: 实现规则引擎 (1-2月)

**目标**: 将复杂的"补"规则配置化

**任务**:
1. ⏳ 设计规则引擎接口
2. ⏳ 实现条件判断引擎
3. ⏳ 实现位置提取引擎
4. ⏳ 迁移"补"规则到配置文件
5. ⏳ 测试规则引擎

**验收标准**:
- [ ] 规则引擎实现完成
- [ ] 80%的"补"规则配置化
- [ ] 功能测试通过

---

## 7. 风险和缓解措施

### 7.1 风险: 配置文件过于复杂

**描述**: 推理规则可能非常复杂,难以用YAML表达

**缓解措施**:
- 从简单规则开始
- 复杂规则保留代码实现
- 逐步迁移,不强求100%配置化

### 7.2 风险: 性能下降

**描述**: 配置驱动可能影响性能

**缓解措施**:
- 使用`sync.Once`单例模式
- 配置文件缓存
- 性能测试和优化

### 7.3 风险: 向后兼容性

**描述**: 新Parser输出可能与旧版本不一致

**缓解措施**:
- Phase 1保持功能不变
- 对比测试 (5000条RSS数据)
- 逐步迁移,保留fallback

---

## 8. 成功指标

| 指标 | 基线 | Phase 1 | Phase 2 | Phase 3 |
|------|------|---------|---------|---------|
| Lexicon覆盖率 | 30% | 30% | 50% | 70% |
| 配置化程度 | 0% | 40% | 60% | 90% |
| "补"机制复杂度 | 高 | 高 | 中 | 低 |
| 新规则添加步骤 | 5步 | 3步 | 2步 | 1步 |
| RSS匹配准确率 | 84% | 84% | 87% | 92% |
| Title清洁度 | 60% | 65% | 80% | 95% |

---

## 9. 参考文档

- [Recognition Mechanism Reality Check](../../training/docs/RECOGNITION_MECHANISM_REALITY_CHECK.md)
- [Implementation Depth Analysis](../../training/docs/IMPLEMENTATION_DEPTH_ANALYSIS.md)
- [Parser Layered Architecture Analysis](../../training/docs/PARSER_LAYERED_ARCHITECTURE_ANALYSIS.md)
- [Unified Schema Design V2](../../training/docs/UNIFIED_SCHEMA_DESIGN_V2.md)
