# Title Parser v2 - 快速开始

**版本**: v2.0.0  
**日期**: 2025-01-17

---

## 项目状态

✅ **Phase 1完成**: 项目结构和配置文件已准备就绪  
⏳ **Phase 2进行中**: 核心组件实现

---

## 项目结构概览

```
title-parser/
├── configs/          # 6个配置文件 (~1700行)
├── docs/             # 7个设计文档
├── specs/            # 3个spec文档
├── pkg/              # 核心包 (待实现)
├── cmd/              # 命令行工具 (待实现)
└── tests/            # 测试 (待实现)
```

---

## 核心配置文件

### 1. attribute_config.yaml (~600行)
- 15个字段定义
- 4个字段组
- 业务规则
- 标准化规则

### 2. template_catalog.yaml (~250行)
- 33个模板定义
- TV_EP: 6个
- BATCH: 11个
- MOVIE: 6个
- SP: 10个

### 3. inference_rules.yaml (~350行)
- Content Type推理规则 (7个)
- Contents降级规则 (5个)
- SP Kind推理规则 (12个)
- Episode推理规则 (6个)
- Title推理规则 (4个)
- Aliases推理规则 (4个)

### 4. boundary_rules.yaml (~250行)
- 强边界定义 (5个)
- 弱边界定义 (3个)
- 标题修饰符 (15个)
- 边界检测规则 (6个)

### 5. lexicon_v1.yaml (~200行)
- Lexicon字典 (~110个规则)
- 技术属性、语言标记、平台标记等

### 6. tailgroup_whitelist.yaml (~50行)
- TAILGROUP白名单

---

## 核心设计

### Content Type优先策略

```
改进前: Lexicon → EP处理 → TITLE推理 → 模板匹配 → Content Kind推理
改进后: Lexicon → 信号检测 → Content Type推理 → 模板匹配(过滤) → 字段提取
```

**关键优势**:
- ✅ Content Type在模板匹配之前确定
- ✅ 避免Season Pack被误识别为TV Episode
- ✅ 模板匹配更准确

### Contents降级机制

```
优先级: Episode/EpisodeRange (最高) > Movie/SP (降级为Contents)

示例: "[VCB-Studio] Shinka no Mi 01-12+Movie+OVA [Ma10p_1080p]"
识别: episode_range=01-12, movie_flag=Movie, special_flag=OVA
降级: movie_flag → contents, special_flag → contents
输出: {
  "content_kind": "BATCH",
  "episode_range": "01-12",
  "contents": ["MOVIE", "OVA"]
}
```

---

## 下一步工作

### Phase 2: 核心组件实现 (本周)

#### 2.1 数据模型定义
**文件**: `pkg/model/types.go`

定义所有数据类型:
- `AttrType` - 属性类型枚举
- `Attribute` - 属性结构
- `Span` - 文本范围
- `ParseResult` - 解析结果

#### 2.2 ConfigLoader实现
**文件**: `pkg/config/loader.go`

实现配置加载:
- 加载所有6个配置文件
- 提供统一的查询接口
- 使用`sync.Once`确保只加载一次

#### 2.3 ContentTypeInferencer实现
**文件**: `pkg/inference/content_type.go`

实现Content Type推理:
- 基于`inference_rules.yaml`
- 支持优先级规则
- 支持条件判断

#### 2.4 BoundaryDetector实现
**文件**: `pkg/boundary/detector.go`

实现边界检测:
- 基于`boundary_rules.yaml`
- 判断强边界、弱边界
- 判断标题修饰符

#### 2.5 TemplateFilter实现
**文件**: `pkg/template/filter.go`

实现模板过滤:
- 基于Content Type过滤
- 支持SP Kind过滤

#### 2.6 ContentsDemoter实现
**文件**: `pkg/contents/demoter.go`

实现Contents降级:
- 基于`inference_rules.yaml`
- 支持优先级规则
- 支持异常处理

---

## 开发指南

### 1. 阅读设计文档

**必读**:
- `docs/IMPLEMENTATION_PLAN.md` - 实施计划
- `specs/design.md` - 架构设计
- `docs/CONTENT_TYPE_FIRST_STRATEGY.md` - Content Type优先策略

**推荐**:
- `docs/CONTENT_TYPE_TEMPLATE_CATALOG.md` - 33个模板详解
- `docs/RECOGNITION_MECHANISM_REALITY_CHECK.md` - 识别机制现实检查
- `docs/PARSER_LAYERED_ARCHITECTURE_ANALYSIS.md` - 分层架构分析

### 2. 理解配置文件

**顺序**:
1. `attribute_config.yaml` - 理解字段定义
2. `template_catalog.yaml` - 理解模板体系
3. `inference_rules.yaml` - 理解推理规则
4. `boundary_rules.yaml` - 理解边界规则

### 3. 开始实现

**顺序**:
1. 数据模型 (`pkg/model/types.go`)
2. ConfigLoader (`pkg/config/loader.go`)
3. ContentTypeInferencer (`pkg/inference/content_type.go`)
4. BoundaryDetector (`pkg/boundary/detector.go`)
5. TemplateFilter (`pkg/template/filter.go`)
6. ContentsDemoter (`pkg/contents/demoter.go`)

### 4. 编写测试

每个组件都需要单元测试:
- 测试覆盖率 > 80%
- 测试所有边缘case
- 测试错误处理

---

## 成功指标

| 指标 | v1基线 | v2目标 |
|------|--------|--------|
| Content Type推理准确率 | 70% | 90% |
| Season Pack识别准确率 | 60% | 95% |
| Contents降级准确率 | 0% | 95% |
| 模板匹配准确率 | 80% | 92% |
| RSS匹配准确率 | 84% | 92% |
| 配置化程度 | 0% | 90% |
| 新规则添加步骤 | 5步 | 1步 |

---

## 参考资源

### 文档
- [README.md](./README.md) - 项目概述
- [IMPLEMENTATION_PLAN.md](./docs/IMPLEMENTATION_PLAN.md) - 实施计划
- [MIGRATION_SUMMARY.md](./docs/MIGRATION_SUMMARY.md) - 迁移总结

### Spec
- [requirements.md](./specs/requirements.md) - 需求文档
- [design.md](./specs/design.md) - 设计文档
- [tasks.md](./specs/tasks.md) - 任务列表

### 配置文件
- [configs/](./configs/) - 所有配置文件

---

## 联系方式

如有问题,请参考文档或提issue。

