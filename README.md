# Title Parser v2 - Config-Driven Architecture

**版本**: v2.0.0  
**创建日期**: 2025-01-17  
**架构**: 配置驱动 + 分层设计

---

## 项目概述

这是Title Parser的v2版本,采用全新的配置驱动架构,旨在解决v1版本的以下问题:
- 硬编码规则分散在代码中
- 添加新规则需要修改多处代码
- "补"机制复杂且难以维护
- Content Type推理在模板匹配之后,导致类型混淆

## 核心改进

### 1. Content Type优先策略
```
v1流程: Lexicon → EP处理 → TITLE推理 → 模板匹配 → Content Kind推理
v2流程: Lexicon → 信号检测 → Content Type推理 → 模板匹配(过滤) → 字段提取
```

**关键优势**:
- ✅ 避免Season Pack被误识别为TV Episode
- ✅ 模板匹配更准确 (只考虑适用的模板)

### 2. 配置驱动架构
所有规则在配置文件中定义:
- `configs/attribute_config.yaml` - 字段定义和业务规则
- `configs/template_catalog.yaml` - 33个模板定义
- `configs/inference_rules.yaml` - Content Type推理和Contents降级规则
- `configs/boundary_rules.yaml` - 边界识别规则
- `configs/lexicon_v1.yaml` - Lexicon字典
- `configs/tailgroup_whitelist.yaml` - TAILGROUP白名单

### 3. 分层架构 (9层)
```
Layer 1: Tokenization (分词)
Layer 2: Lexicon Matching (字典匹配) ← 配置驱动
Layer 3: Signal Detection (信号识别)
Layer 4: Content Type Inference (类型推理) ← 在模板匹配之前
Layer 5: Template Matching (模板匹配) ← 基于Content Type过滤
Layer 6: Field Extraction (字段提取) ← 配置驱动
Layer 7: Inference (推理) ← 配置驱动
Layer 8: Cleaning (清理) ← 配置驱动
Layer 9: Output Building (输出构建)
```

### 4. Contents降级机制
```
优先级: Episode/EpisodeRange (最高) > Movie/SP (降级为Contents)

示例: "[VCB-Studio] Shinka no Mi 01-12+Movie+OVA [Ma10p_1080p]"
  → episode_range=01-12, contents=["MOVIE", "OVA"]
```

## 项目结构

```
title-parser/
├── README.md                    # 本文件
├── go.mod                       # Go模块定义
├── go.sum                       # Go依赖锁定
├── configs/                     # 配置文件目录
│   ├── attribute_config.yaml
│   ├── template_catalog.yaml
│   ├── inference_rules.yaml
│   ├── boundary_rules.yaml
│   ├── lexicon_v1.yaml
│   └── tailgroup_whitelist.yaml
├── pkg/                         # 核心包
│   ├── config/                  # 配置加载
│   │   ├── loader.go
│   │   ├── loader_test.go
│   │   └── types.go
│   ├── inference/               # 推理引擎
│   │   ├── content_type.go
│   │   ├── content_type_test.go
│   │   ├── sp_kind.go
│   │   ├── episode.go
│   │   ├── title.go
│   │   └── aliases.go
│   ├── boundary/                # 边界检测
│   │   ├── detector.go
│   │   ├── detector_test.go
│   │   └── types.go
│   ├── template/                # 模板匹配
│   │   ├── filter.go
│   │   ├── filter_test.go
│   │   ├── matcher.go
│   │   └── types.go
│   ├── contents/                # Contents降级
│   │   ├── demoter.go
│   │   ├── demoter_test.go
│   │   └── types.go
│   ├── parser/                  # 主Parser
│   │   ├── parser.go
│   │   ├── parser_test.go
│   │   └── types.go
│   └── model/                   # 数据模型
│       ├── attribute.go
│       ├── segment.go
│       ├── result.go
│       └── types.go
├── cmd/                         # 命令行工具
│   ├── parse/                   # 解析工具
│   │   └── main.go
│   └── validate/                # 验证工具
│       └── main.go
├── docs/                        # 文档
│   ├── ARCHITECTURE.md
│   ├── CONFIG_GUIDE.md
│   ├── MIGRATION_FROM_V1.md
│   └── API_REFERENCE.md
└── tests/                       # 集成测试
    ├── integration_test.go
    └── testdata/
```

## 快速开始

### 安装依赖
```bash
cd title-parser
go mod download
```


### 编译

**推荐方式** (只编译需要的包):
```bash
# 编译 HTTP 服务器
go build -o server ./cmd/server

# 或使用 go run (无需编译)
go run ./cmd/server
```

**注意**: 不要使用 `go build ./...` 或 `go build .` - 这会尝试编译所有包。

### 运行测试
```bash
go test ./...
```


### 使用示例
```go
package main

import (
    "fmt"
    "github.com/your-org/title-parser/pkg/config"
    "github.com/your-org/title-parser/pkg/parser"
)

func main() {
    // 加载配置
    cfg, err := config.LoadAll("configs")
    if err != nil {
        panic(err)
    }
    
    // 创建Parser
    p := parser.New(cfg)
    
    // 解析标题
    result, err := p.Parse("[LoliHouse] 药屋少女 - 01 [WebRip 1080p HEVC AAC]")
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Title: %s\n", result.Title)
    fmt.Printf("Episode: %s\n", result.Episode)
    fmt.Printf("Content Kind: %s\n", result.ContentKind)
}
```

## 开发计划

### Phase 1: 核心组件实现 (本周) ⏳
- [ ] ConfigLoader实现
- [ ] ContentTypeInferencer实现
- [ ] BoundaryDetector实现
- [ ] TemplateFilter实现
- [ ] ContentsDemoter实现

### Phase 2: Parser集成 (1周)
- [ ] Parser主流程实现
- [ ] 功能一致性测试
- [ ] 性能测试

### Phase 3: Lexicon扩展 (1-2周)
- [ ] 补充平台标记 (~20个)
- [ ] 补充地区标记 (~20个)
- [ ] 补充内容分级 (~10个)

### Phase 4: 规则引擎 (1-2月)
- [ ] 规则引擎设计
- [ ] 规则引擎实现
- [ ] 迁移"补"规则

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

## 添加新规则

当发现未识别的格式/模板/属性/词典时:

📖 **快速参考**: [QUICK_REFERENCE.md](./QUICK_REFERENCE.md) - 5分钟快速上手  
📚 **详细指南**: [ADDING_NEW_RULES_GUIDE.md](./docs/ADDING_NEW_RULES_GUIDE.md) - 完整操作流程

### 快速决策

| 发现了什么? | 修改哪个文件? | 时间 |
|-----------|-------------|------|
| 新的技术属性/平台 | `configs/lexicon_v1.yaml` | 5-10分钟 |
| 新的模板格式 | `configs/template_catalog.yaml` | 15-30分钟 |
| 新的字段类型 | `configs/attribute_config.yaml` + 代码 | 30-60分钟 |
| 新的Content Type | `configs/inference_rules.yaml` + 代码 | 60-90分钟 |
| 新的边界规则 | `configs/boundary_rules.yaml` | 10-20分钟 |

**v1 vs v2**: 添加新词典项从5步30分钟减少到2步5分钟 (改进80%)

## 参考文档

- [Quick Reference](./QUICK_REFERENCE.md) - 快速参考卡片
- [Adding New Rules Guide](./docs/ADDING_NEW_RULES_GUIDE.md) - 添加新规则指南
- [Implementation Plan](./docs/IMPLEMENTATION_PLAN.md) - 实施计划
- [Migration Summary](./docs/MIGRATION_SUMMARY.md) - 迁移总结
- [Complex Alias Patterns](./docs/COMPLEX_ALIAS_PATTERNS.md) - 复杂Alias拆分和NOTE/NOISE处理
- [Alias and Noise Handling Strategy](./docs/ALIAS_AND_NOISE_HANDLING_STRATEGY.md) - 完整的处理策略

## 从v1迁移

详见 [MIGRATION_FROM_V1.md](./docs/MIGRATION_FROM_V1.md)


## 已知问题

### 1. 编译问题 (已解决)

**问题**: `go build ./...` 失败，提示 `pkg/onnx/token_boundary.go` 导入不存在的包。

**原因**: 项目中存在两个独立的 Go module：
- `title-parser/` (module: `title-parser`)
- `title-parser/training/go/titleparser/` (module: `reltitle/training/titleparser`)

`pkg/onnx` 试图通过相对路径导入子 module 的包，这在 Go 中不可行。

**解决方案 (已完成)**:
- 删除 `pkg/onnx` 目录
- 删除过时的测试文件 `test_parse_with_hints.go`

### 2. 编译注意事项

- 使用 `go build -o server ./cmd/server` 编译服务器
- 或使用 `go run ./cmd/server` 直接运行
- **不要使用** `go build ./...` 或 `go build .` - 会尝试编译存在问题的测试文件

### 3. 与 downstream 集成

title-parser 作为独立服务运行，通过 HTTP API 与 downstream backend 通信。

**相关文件**:
- `downstream/backend/src/services/parser.service.ts` - Backend 解析服务
- `downstream/backend/src/services/matching/matching-pipeline.service.ts` - 匹配管道

## License

MIT

