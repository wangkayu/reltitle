# 配置驱动Parser重构 - 任务列表

**Feature Name**: config-driven-parser-refactor  
**Created**: 2025-01-17  
**Status**: In Progress - Phase 1  
**Priority**: P0

---

## Phase 1: 配置文件创建 (本周) ✅ 已完成

### 任务列表

- [x] 创建`attribute_config.yaml` - 字段定义和业务规则
- [x] 创建`template_catalog.yaml` - 33个模板定义
- [x] 创建`inference_rules.yaml` - Content Type推理和Contents降级规则
- [x] 创建`boundary_rules.yaml` - 边界识别规则
- [x] 文档编写 - `CONFIG_FILES_CREATION_COMPLETE.md`

### 完成时间
2025-01-17

### 验收标准
- [x] 所有配置文件创建完成 (6个文件, ~1700行)
- [x] 配置文件包含完整的规则定义
- [x] 配置文件有清晰的注释和示例
- [x] 文档记录配置文件的使用方法

---

## Phase 2: 实现ConfigLoader和核心组件 (本周) ⏳ 进行中

### 任务列表

#### 2.1 ConfigLoader实现
- [ ] 实现`ConfigLoader`接口
  - [ ] `LoadAll()` - 加载所有配置文件
  - [ ] `GetFieldConfig()` - 查询字段配置
  - [ ] `GetBoundaryRules()` - 查询边界规则
  - [ ] `GetInferenceRules()` - 查询推理规则
  - [ ] `GetBusinessRule()` - 查询业务规则
- [ ] 实现`FileConfigLoader`
  - [ ] 使用`sync.Once`确保只加载一次
  - [ ] 支持YAML解析
  - [ ] 错误处理和验证
- [ ] 单元测试
  - [ ] 测试配置加载
  - [ ] 测试配置查询
  - [ ] 测试错误处理

#### 2.2 ContentTypeInferencer实现
- [ ] 实现`ContentTypeInferencer`接口
  - [ ] `Infer()` - 推理Content Type
  - [ ] `checkCondition()` - 检查条件
  - [ ] 支持优先级规则
- [ ] 实现条件判断引擎
  - [ ] `has_episode`, `no_episode`
  - [ ] `has_season`, `has_tech`
  - [ ] `has_movie_keyword`, `has_sp_kind`
- [ ] 单元测试
  - [ ] 测试TV_EP推理
  - [ ] 测试BATCH推理 (Season Pack)
  - [ ] 测试MOVIE推理
  - [ ] 测试SPECIAL推理

#### 2.3 BoundaryDetector实现
- [ ] 实现`BoundaryDetector`接口
  - [ ] `IsStrongBoundary()` - 判断强边界
  - [ ] `IsTitleModifier()` - 判断标题修饰符
  - [ ] `IsTokenSurroundedByStrongBoundaries()` - 判断token是否被包围
  - [ ] `FilterTitleTokensByBoundaries()` - 过滤title tokens
- [ ] 实现`ConfigBoundaryDetector`
  - [ ] 从配置文件加载规则
  - [ ] 支持日文、英文、中文修饰符
  - [ ] 支持case_insensitive匹配
- [ ] 单元测试
  - [ ] 测试强边界识别
  - [ ] 测试弱边界识别
  - [ ] 测试标题修饰符识别

#### 2.4 TemplateFilter实现
- [ ] 实现`TemplateFilter`接口
  - [ ] `FilterByContentType()` - 基于Content Type过滤
  - [ ] `FilterBySPKind()` - 基于SP Kind过滤
- [ ] 实现模板加载
  - [ ] 从`template_catalog.yaml`加载模板
  - [ ] 解析模板定义
  - [ ] 构建模板索引
- [ ] 单元测试
  - [ ] 测试Content Type过滤
  - [ ] 测试SP Kind过滤
  - [ ] 测试模板匹配

#### 2.5 ContentsDemoter实现
- [ ] 实现`ContentsDemoter`接口
  - [ ] `Demote()` - 降级Contents
  - [ ] `checkCondition()` - 检查条件
  - [ ] `shouldSkipDemotion()` - 检查异常
- [ ] 实现降级逻辑
  - [ ] MovieFlag → Contents
  - [ ] SpecialFlag → Contents
  - [ ] 支持优先级规则
  - [ ] 支持异常处理 (S00Exx, overlap)
- [ ] 单元测试
  - [ ] 测试Episode + Movie降级
  - [ ] 测试EpisodeRange + Movie降级
  - [ ] 测试Season + Movie降级
  - [ ] 测试异常处理

### 验收标准
- [ ] 所有核心组件实现完成
- [ ] 单元测试覆盖率 > 80%
- [ ] 所有测试通过
- [ ] 代码review通过

---

## Phase 3: 集成到Parser (1周) ⏳ 待开始

### 任务列表

#### 3.1 Parser流程重构
- [ ] 重构`Parse()`函数
  - [ ] 添加Content Type推理步骤 (在模板匹配之前)
  - [ ] 使用`ContentTypeInferencer`推理Content Type
  - [ ] 使用`TemplateFilter`过滤模板
  - [ ] 使用`ContentsDemoter`降级Contents
- [ ] 重构边界识别
  - [ ] 使用`BoundaryDetector`替换硬编码
  - [ ] 更新`slot_boundary.go`
  - [ ] 更新相关函数
- [ ] 保持向后兼容
  - [ ] 保留fallback逻辑
  - [ ] 支持配置开关

#### 3.2 功能一致性测试
- [ ] 对比测试
  - [ ] 测试5000条RSS数据
  - [ ] 对比新旧Parser输出
  - [ ] 记录不一致的case
- [ ] 修复不一致
  - [ ] 分析不一致原因
  - [ ] 调整配置或代码
  - [ ] 重新测试
- [ ] 回归测试
  - [ ] 运行所有现有测试
  - [ ] 确保测试通过

#### 3.3 性能测试
- [ ] 基准测试
  - [ ] 测试配置加载性能
  - [ ] 测试推理性能
  - [ ] 测试整体Parser性能
- [ ] 性能优化
  - [ ] 识别性能瓶颈
  - [ ] 优化热点代码
  - [ ] 添加缓存
- [ ] 性能验证
  - [ ] 确保性能损失 < 10%
  - [ ] 记录性能指标

### 验收标准
- [ ] 新Parser输出与旧Parser一致 (>95%)
- [ ] RSS匹配准确率不降低
- [ ] 性能不降低 (<10%性能损失)
- [ ] 所有测试通过

---

## Phase 4: 补充Lexicon规则 (1-2周) ⏳ 待开始

### 任务列表

#### 4.1 补充平台标记 (~20个)
- [ ] 研究常见平台
  - [ ] Netflix, Bilibili, Crunchyroll等
  - [ ] 收集平台标记变体
- [ ] 添加到`lexicon_v1.yaml`
  - [ ] 定义平台规则
  - [ ] 添加normalized值
- [ ] 测试
  - [ ] 测试平台识别
  - [ ] 验证准确率

#### 4.2 补充地区标记 (~20个)
- [ ] 研究常见地区限制
  - [ ] 港澳台、大陆、日本等
  - [ ] 收集地区标记变体
- [ ] 添加到`lexicon_v1.yaml`
  - [ ] 定义地区规则
  - [ ] 添加normalized值
- [ ] 测试
  - [ ] 测试地区识别
  - [ ] 验证准确率

#### 4.3 补充内容分级 (~10个)
- [ ] 研究常见内容分级
  - [ ] uncensored, censored等
  - [ ] 收集分级标记变体
- [ ] 添加到`lexicon_v1.yaml`
  - [ ] 定义分级规则
  - [ ] 添加normalized值
- [ ] 测试
  - [ ] 测试分级识别
  - [ ] 验证准确率

#### 4.4 补充版本标记 (~10个)
- [ ] 研究常见版本标记
  - [ ] v2, repack, proper等
  - [ ] 收集版本标记变体
- [ ] 添加到`lexicon_v1.yaml`
  - [ ] 定义版本规则
  - [ ] 添加normalized值
- [ ] 测试
  - [ ] 测试版本识别
  - [ ] 验证准确率

#### 4.5 补充完结标记 (~10个)
- [ ] 研究常见完结标记
  - [ ] 完结、完結、Fin等
  - [ ] 收集完结标记变体
- [ ] 添加到`lexicon_v1.yaml`
  - [ ] 定义完结规则
  - [ ] 添加normalized值
- [ ] 测试
  - [ ] 测试完结识别
  - [ ] 验证准确率

### 验收标准
- [ ] Lexicon规则数量 > 180
- [ ] Lexicon覆盖率 > 50%
- [ ] 新规则测试通过
- [ ] RSS匹配准确率提升

---

## Phase 5: 规则引擎实现 (1-2月) ⏳ 待开始

### 任务列表

#### 5.1 规则引擎设计
- [ ] 设计规则引擎接口
  - [ ] 条件判断引擎
  - [ ] 位置提取引擎
  - [ ] 规则执行引擎
- [ ] 设计规则DSL
  - [ ] 条件表达式语法
  - [ ] 位置表达式语法
  - [ ] 动作表达式语法

#### 5.2 规则引擎实现
- [ ] 实现条件判断引擎
  - [ ] 支持逻辑运算 (AND, OR, NOT)
  - [ ] 支持字段检查 (has_field, no_field)
  - [ ] 支持值比较 (equals, contains, matches)
- [ ] 实现位置提取引擎
  - [ ] 支持位置表达式 (after_X, before_Y, between_X_and_Y)
  - [ ] 支持span操作
  - [ ] 支持过滤和转换
- [ ] 实现规则执行引擎
  - [ ] 支持规则优先级
  - [ ] 支持规则链
  - [ ] 支持fallback

#### 5.3 迁移"补"规则
- [ ] 迁移位置推理规则
  - [ ] between_group_and_episode
  - [ ] between_episode_and_tech
  - [ ] from_tail
- [ ] 迁移模板家族规则
  - [ ] TF02规则
  - [ ] TF03规则
  - [ ] 其他模板规则
- [ ] 迁移Group特定规则
  - [ ] LoliHouse规则
  - [ ] Nekomoe规则
  - [ ] 其他Group规则

#### 5.4 测试和验证
- [ ] 单元测试
  - [ ] 测试条件判断
  - [ ] 测试位置提取
  - [ ] 测试规则执行
- [ ] 集成测试
  - [ ] 测试规则引擎与Parser集成
  - [ ] 测试5000条RSS数据
  - [ ] 对比准确率
- [ ] 性能测试
  - [ ] 测试规则引擎性能
  - [ ] 优化性能瓶颈

### 验收标准
- [ ] 规则引擎实现完成
- [ ] 80%的"补"规则配置化
- [ ] 功能测试通过
- [ ] 性能不降低

---

## 成功指标

| 指标 | 基线 | Phase 1 | Phase 2 | Phase 3 | Phase 4 | Phase 5 |
|------|------|---------|---------|---------|---------|---------|
| 配置化程度 | 0% | 20% | 40% | 60% | 70% | 90% |
| Content Type推理准确率 | 70% | 70% | 75% | 85% | 87% | 90% |
| Season Pack识别准确率 | 60% | 60% | 70% | 85% | 90% | 95% |
| Contents降级准确率 | 0% | 0% | 80% | 90% | 92% | 95% |
| 模板匹配准确率 | 80% | 80% | 82% | 87% | 89% | 92% |
| Lexicon覆盖率 | 30% | 30% | 30% | 40% | 50% | 70% |
| 新规则添加步骤 | 5步 | 5步 | 3步 | 2步 | 2步 | 1步 |
| RSS匹配准确率 | 84% | 84% | 84% | 87% | 89% | 92% |

---

## 风险和缓解措施

### 风险1: 配置文件过于复杂
**描述**: 推理规则可能非常复杂,难以用YAML表达

**缓解措施**:
- 从简单规则开始
- 复杂规则保留代码实现
- 逐步迁移,不强求100%配置化

### 风险2: 性能下降
**描述**: 配置驱动可能影响性能

**缓解措施**:
- 使用`sync.Once`单例模式
- 配置文件缓存
- 性能测试和优化

### 风险3: 向后兼容性
**描述**: 新Parser输出可能与旧版本不一致

**缓解措施**:
- Phase 1保持功能不变
- 对比测试 (5000条RSS数据)
- 逐步迁移,保留fallback

---

## 参考文档

- [Config Files Creation Complete](../../training/docs/CONFIG_FILES_CREATION_COMPLETE.md)
- [Content Type Template Catalog](../../training/docs/CONTENT_TYPE_TEMPLATE_CATALOG.md)
- [Content Type First Strategy](../../training/docs/CONTENT_TYPE_FIRST_STRATEGY.md)
- [Recognition Mechanism Reality Check](../../training/docs/RECOGNITION_MECHANISM_REALITY_CHECK.md)
- [Config Driven Parser Refactor - Design](./design.md)
- [Config Driven Parser Refactor - Requirements](./requirements.md)

