# 配置驱动Parser重构 - 需求文档

**Feature Name**: config-driven-parser-refactor  
**Created**: 2025-01-17  
**Status**: Draft  
**Priority**: P0 (核心架构改进)

---

## 1. 业务背景

### 1.1 当前问题

我们的Title Parser存在以下核心问题:

1. **字段逐步添加导致不独立** - 新字段依赖旧字段,修改一个字段影响其他字段
2. **规则入口过多** - 添加新属性需要修改5个地方 (model/types.go, lexicon_v1.yaml, parser.go, CRF训练, 白名单)
3. **表层功能多** - 很多功能定义了但没有深度集成 (平均深度只有52.5%)
4. **字段混淆** - Platform, Region等应该是独立字段,但混在note中
5. **单体巨大** - parser.go有6868行,难以维护和测试

### 1.2 业务影响

- **RSS匹配准确率**: 当前84%,目标92%
- **Title清洁度**: 当前60%,目标95% (分隔符残留影响embedding匹配)
- **开发效率**: 添加新字段需要5步,目标2步 (效率提升60%)
- **维护成本**: 单体代码难以维护,bug修复周期长

### 1.3 核心业务流程

**RSS标题匹配流程**:
```
RSS Raw Title → Parser → query_text → Embedding匹配 → Top-K候选 → Rerank → AniDB ID
```

**关键需求**:
- 干净的title用于主匹配 (无分隔符残留)
- 多个aliases用于提高召回率
- Metadata用于Rerank验证

---

## 2. 用户故事

### 2.1 作为开发者,我希望添加新字段时只需修改配置文件

**当前流程** (5步):
1. 在`model/types.go`中定义字段类型
2. 在`lexicon_v1.yaml`中添加pattern
3. 在`parser.go`中添加提取逻辑
4. 在CRF训练数据中添加标注
5. 在白名单中添加规则

**期望流程** (2步):
1. 在`attribute_config.yaml`中定义字段
2. 运行测试验证

**验收标准**:
- [ ] 新字段定义在配置文件中
- [ ] 配置加载器自动生成提取器
- [ ] 无需修改Go代码
- [ ] 测试覆盖率 > 90%

### 2.2 作为开发者,我希望每个字段独立计算,不依赖其他字段

**当前问题**:
- `title_variants`依赖`title`的清洁度
- `content_kind`依赖`episode`的准确性
- 修改一个字段可能破坏其他字段

**期望行为**:
- 每个字段有独立的提取器
- 字段之间的依赖关系在配置中明确定义
- 修改一个字段不影响其他字段

**验收标准**:
- [ ] 所有字段有独立的提取器
- [ ] 字段依赖关系在配置中定义
- [ ] 字段独立性测试通过 (100%)

### 2.3 作为RSS匹配系统,我需要干净的title用于embedding匹配

**当前问题**:
- Title有分隔符残留: `药屋少女 / /`
- Tech noise未完全清理
- 影响embedding匹配准确率

**期望行为**:
- Title无分隔符残留
- Title无技术属性噪音
- Title清洁度 > 95%

**验收标准**:
- [ ] Title不以分隔符结尾 (`/`, `|`, `｜`)
- [ ] Title不包含tech属性 (1080p, WebRip等)
- [ ] Title清洁度测试通过 (> 95%)
- [ ] RSS匹配准确率提升 (84% → 92%)

### 2.4 作为开发者,我希望Platform, Region等字段独立,不混在note中

**当前问题**:
- Platform作为note (kind=PLATFORM)
- Region硬编码在parser中
- Content Rating在Lexicon有定义但parser不处理

**期望行为**:
- Platform是独立字段,有明确的枚举值
- Region是独立字段,可配置
- Content Rating是独立字段,从Lexicon提取

**验收标准**:
- [ ] Platform字段独立输出
- [ ] Region字段独立输出
- [ ] Content Rating字段独立输出
- [ ] 所有字段有明确的业务用途

### 2.5 作为开发者,我希望Parser代码分层清晰,易于维护

**当前问题**:
- parser.go有6868行
- 所有逻辑混在一起
- 难以测试和维护

**期望行为**:
- Parser分为7层,每层职责清晰
- 每层可以独立测试
- 总代码量不变,但结构清晰

**验收标准**:
- [ ] parser.go < 500行 (主流程)
- [ ] 7个layer,每个500-1200行
- [ ] 每层有独立的测试
- [ ] 测试覆盖率 > 80%

---

## 3. 功能需求

### 3.1 配置文件系统

**FR-1.1**: 创建`attribute_config.yaml`配置文件

**描述**: 所有字段定义、业务规则、标准化规则都在配置文件中

**包含内容**:
- 字段定义 (name, type, required, business_priority, description)
- 提取规则 (source, method, post_process)
- 验证规则 (pattern, range, length)
- 标准化规则 (mapping, format, trim)
- 业务规则 (rss_matching, quality_check, content_kind_inference)
- 字段依赖关系 (independent_fields, derived_fields)

**验收标准**:
- [ ] 配置文件包含所有核心字段 (title, episode, content_kind等)
- [ ] 配置文件包含所有技术属性 (source_type, resolution等)
- [ ] 配置文件包含所有元数据字段 (qualified, confidence等)
- [ ] 配置文件格式正确,可以被YAML解析器解析

**FR-1.2**: 实现配置加载器

**描述**: 加载配置文件,提供配置查询接口

**功能**:
- 加载YAML配置文件
- 解析字段定义
- 解析业务规则
- 解析标准化规则
- 提供配置查询接口 (GetFieldConfig, GetBusinessRule等)

**验收标准**:
- [ ] 配置加载器可以解析attribute_config.yaml
- [ ] 配置加载器提供查询接口
- [ ] 配置加载器有错误处理 (文件不存在、格式错误等)
- [ ] 配置加载器有单元测试

### 3.2 字段提取系统

**FR-2.1**: 实现字段提取器接口

**描述**: 定义统一的字段提取器接口

```go
type FieldExtractor interface {
    Extract(ctx ExtractionContext) (interface{}, error)
}

type ExtractionContext struct {
    RawTitle  string
    Slots     Slots
    Config    FieldConfig
}
```

**验收标准**:
- [ ] 定义FieldExtractor接口
- [ ] 定义ExtractionContext结构
- [ ] 定义ExtractorRegistry注册机制

**FR-2.2**: 实现核心字段提取器

**描述**: 为每个核心字段实现独立的提取器

**字段列表**:
- TitleExtractor - 提取title
- TitleVariantsExtractor - 提取title_variants
- EpisodeExtractor - 提取episode
- EpisodeRangeExtractor - 提取episode_range
- SeasonExtractor - 提取season
- GroupExtractor - 提取group

**验收标准**:
- [ ] 每个字段有独立的提取器
- [ ] 提取器基于配置文件工作
- [ ] 提取器有单元测试
- [ ] 提取器测试覆盖率 > 90%

**FR-2.3**: 实现技术属性提取器

**描述**: 为每个技术属性实现独立的提取器

**字段列表**:
- SourceTypeExtractor - 提取source_type
- ResolutionExtractor - 提取resolution
- VideoCodecExtractor - 提取video_codec
- BitDepthExtractor - 提取bit_depth
- AudioCodecExtractor - 提取audio_codec

**验收标准**:
- [ ] 每个技术属性有独立的提取器
- [ ] 提取器基于配置文件的字典匹配
- [ ] 提取器有单元测试

### 3.3 标准化系统

**FR-3.1**: 实现标准化器接口

**描述**: 定义统一的标准化器接口

```go
type Standardizer interface {
    Standardize(value interface{}, config StandardizationConfig) (interface{}, error)
}
```

**验收标准**:
- [ ] 定义Standardizer接口
- [ ] 定义StandardizationConfig结构

**FR-3.2**: 实现标准化器

**描述**: 实现各种标准化器

**标准化器列表**:
- MappingStandardizer - 映射标准化 (WEBRIP → WebRip)
- FormatStandardizer - 格式标准化 (10 → 10bit)
- TrimStandardizer - 去除首尾空格
- CollapseSpacesStandardizer - 合并多个空格
- RemoveTrailingSeparatorsStandardizer - 去除尾部分隔符

**验收标准**:
- [ ] 所有标准化器实现
- [ ] 标准化器基于配置文件工作
- [ ] 标准化器有单元测试

### 3.4 验证系统

**FR-4.1**: 实现验证器接口

**描述**: 定义统一的验证器接口

```go
type Validator interface {
    Validate(value interface{}, config ValidationConfig) error
}
```

**验收标准**:
- [ ] 定义Validator接口
- [ ] 定义ValidationConfig结构

**FR-4.2**: 实现验证器

**描述**: 实现各种验证器

**验证器列表**:
- PatternValidator - 正则表达式验证
- RangeValidator - 范围验证
- LengthValidator - 长度验证
- EnumValidator - 枚举值验证

**验收标准**:
- [ ] 所有验证器实现
- [ ] 验证器基于配置文件工作
- [ ] 验证器有单元测试

### 3.5 推理系统

**FR-5.1**: 实现推理器接口

**描述**: 定义统一的推理器接口

```go
type Inferencer interface {
    Infer(ctx InferenceContext) (Result, Confidence)
}

type InferenceContext struct {
    Fields    Fields
    Slots     Slots
    Template  Template
    Config    InferenceConfig
}
```

**验收标准**:
- [ ] 定义Inferencer接口
- [ ] 定义InferenceContext结构

**FR-5.2**: 实现Content Kind推理器

**描述**: 基于配置文件的推理规则推理content_kind

**推理规则** (从配置文件):
- has_episode && !has_episode_range → TV_EP (confidence: 0.9)
- has_episode_range → BATCH (confidence: 0.95)
- has_movie_keyword → MOVIE (confidence: 0.85)
- has_ova_keyword → OVA (confidence: 0.85)

**验收标准**:
- [ ] Content Kind推理器实现
- [ ] 推理器基于配置文件的规则
- [ ] 推理器有单元测试
- [ ] 推理准确率 > 90%

### 3.6 独立字段实现

**FR-6.1**: Platform字段独立

**描述**: Platform从note中独立出来,成为一级字段

**当前**: note (kind=PLATFORM)  
**目标**: 独立字段

**验收标准**:
- [ ] Platform在配置文件中定义
- [ ] Platform有独立的提取器
- [ ] Platform有明确的枚举值 (NETFLIX, BILIBILI, CRUNCHYROLL等)
- [ ] Platform在输出中独立显示

**FR-6.2**: Region字段独立

**描述**: Region从硬编码变为配置驱动

**当前**: 硬编码在parser中  
**目标**: 配置驱动

**验收标准**:
- [ ] Region在配置文件中定义
- [ ] Region有独立的提取器
- [ ] Region有明确的枚举值 (HK_TW_MO, CN_ONLY等)
- [ ] Region在输出中独立显示

**FR-6.3**: Content Rating字段实现

**描述**: Content Rating从缺失到实现

**当前**: Lexicon有定义,parser不处理  
**目标**: 完整实现

**验收标准**:
- [ ] Content Rating在配置文件中定义
- [ ] Content Rating有独立的提取器
- [ ] Content Rating有明确的枚举值 (UNCENSORED, CENSORED等)
- [ ] Content Rating在输出中显示

---

## 4. 非功能需求

### 4.1 性能需求

**NFR-1.1**: 解析性能不降低

**描述**: 配置驱动不应该降低解析性能

**验收标准**:
- [ ] 单次解析时间 < 10ms (与当前相同)
- [ ] 配置加载使用单例模式,只加载一次
- [ ] 性能测试通过 (1000次解析 < 10秒)

### 4.2 可维护性需求

**NFR-2.1**: 代码结构清晰

**描述**: Parser代码分层清晰,易于维护

**验收标准**:
- [ ] parser.go < 500行
- [ ] 每个layer < 1200行
- [ ] 代码复杂度 < 15 (cyclomatic complexity)

**NFR-2.2**: 测试覆盖率高

**描述**: 所有新代码有充分的测试

**验收标准**:
- [ ] 单元测试覆盖率 > 80%
- [ ] 集成测试覆盖核心流程
- [ ] 所有边缘案例有测试

### 4.3 可扩展性需求

**NFR-3.1**: 易于添加新字段

**描述**: 添加新字段只需修改配置文件

**验收标准**:
- [ ] 新字段添加步骤 < 3步
- [ ] 新字段不需要修改Go代码
- [ ] 新字段自动集成到输出

**NFR-3.2**: 支持混合策略

**描述**: 未来可以整合CRF

**验收标准**:
- [ ] 提取器接口支持多个source
- [ ] 配置文件支持混合策略定义
- [ ] 预留CRF集成接口

### 4.4 向后兼容性需求

**NFR-4.1**: 输出格式兼容

**描述**: 新Parser输出格式与旧Parser兼容

**验收标准**:
- [ ] 核心字段名称不变
- [ ] 核心字段类型不变
- [ ] 下游系统无需修改

---

## 5. 约束条件

### 5.1 技术约束

- 使用Go语言实现
- 使用YAML作为配置文件格式
- 保持与现有系统的兼容性

### 5.2 时间约束

- Phase 1 (配置驱动): 本周完成
- Phase 2 (字段独立化): 1-2周完成
- Phase 3 (混合策略): 1-2月完成

### 5.3 资源约束

- 单人开发
- 不影响现有功能
- 逐步迁移,不是一次性切换

---

## 6. 成功指标

| 指标 | 基线 | 目标 | 优先级 |
|------|------|------|--------|
| 字段独立性 | 60% | 100% | P0 |
| 配置覆盖率 | 0% | 100% | P0 |
| 新字段添加步骤 | 5步 | 2步 | P0 |
| RSS匹配准确率 | 84% | 92% | P0 |
| Title清洁度 | 60% | 95% | P0 |
| 标准化一致性 | 70% | 100% | P1 |
| 代码行数 (parser.go) | 6868行 | < 500行 | P1 |
| 测试覆盖率 | 50% | > 80% | P1 |

---

## 7. 依赖关系

### 7.1 依赖的现有功能

- Slot解析系统 (已完成)
- Template匹配系统 (已完成)
- Lexicon字典系统 (已完成)

### 7.2 被依赖的系统

- RSS匹配系统 (依赖干净的title)
- Rerank系统 (依赖metadata)
- 用户展示系统 (依赖所有字段)

---

## 8. 风险和缓解措施

### 8.1 风险: 配置加载性能

**描述**: 每次Parse都加载配置文件可能影响性能

**缓解措施**:
- 使用`sync.Once`单例模式
- 配置文件缓存
- 热重载机制

### 8.2 风险: 向后兼容性

**描述**: 新Parser输出格式可能与旧系统不兼容

**缓解措施**:
- 保留旧Parser作为fallback
- 实现输出格式转换器
- 逐步迁移,不是一次性切换

### 8.3 风险: 测试覆盖不足

**描述**: 重构可能引入新的bug

**缓解措施**:
- 先写测试,再重构
- 保持功能不变,只改结构
- 使用现有的5000条RSS数据测试

---

## 9. 参考文档

- [实现深度分析](../../training/docs/IMPLEMENTATION_DEPTH_ANALYSIS.md)
- [Parser分层架构分析](../../training/docs/PARSER_LAYERED_ARCHITECTURE_ANALYSIS.md)
- [统一Schema设计 V2.0](../../training/docs/UNIFIED_SCHEMA_DESIGN_V2.md)
- [配置驱动实施进度](../../training/docs/CONFIG_DRIVEN_IMPLEMENTATION_PROGRESS.md)
- [RSS匹配流程分析](../../training/docs/RSS_MATCHING_FLOW_ANALYSIS.md)

---

## 10. 附录

### 10.1 术语表

- **配置驱动**: 通过配置文件管理字段定义和业务规则,而不是硬编码在代码中
- **字段独立性**: 每个字段独立计算,不依赖其他字段
- **Title清洁度**: Title中无分隔符残留、无tech noise的比例
- **RSS匹配准确率**: RSS标题匹配到正确AniDB ID的比例

### 10.2 示例

**示例1: 当前title提取问题**
```
Input: "[LoliHouse] 药屋少女 / Kusuriya no Hitorigoto - 01 [WebRip 1080p]"
Current Output: "药屋少女 / /"  ❌ (分隔符残留)
Expected Output: "药屋少女"  ✅
```

**示例2: 配置驱动的字段定义**
```yaml
fields:
  title:
    category: core
    type: string
    required: true
    extraction:
      source: title_slot
      post_process:
        - trim_delimiters: " /|｜"
        - remove_tech_noise: true
    validation:
      not_ends_with: ["/", "|", "｜"]
```

**示例3: 独立字段提取**
```go
// 每个字段独立提取
titleExtractor := NewTitleExtractor(config)
title := titleExtractor.Extract(ctx)

episodeExtractor := NewEpisodeExtractor(config)
episode := episodeExtractor.Extract(ctx)

// 不依赖其他字段
```
