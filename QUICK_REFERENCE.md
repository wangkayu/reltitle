# 快速参考 - 添加新规则

**版本**: v2.0.0  
**详细指南**: [ADDING_NEW_RULES_GUIDE.md](./docs/ADDING_NEW_RULES_GUIDE.md)

---

## 🎯 快速决策

| 发现了什么? | 修改哪个文件? | 需要改代码? | 时间 |
|-----------|-------------|-----------|------|
| 新的技术属性 (AV1, Opus) | `lexicon.yaml` | ❌ 否 | 5-10分钟 |
| 新的平台 (Disney+) | `lexicon.yaml` | ❌ 否 | 5-10分钟 |
| 新的模板格式 | `template_catalog.yaml` | ❌ 否 | 15-30分钟 |
| 新的字段类型 | `attribute_config.yaml` + 代码 | ✅ 是 | 30-60分钟 |
| 新的Content Type | `inference_rules.yaml` + 代码 | ✅ 是 | 60-90分钟 |
| 新的边界规则 | `boundary_rules.yaml` | ❌ 否 | 10-20分钟 |

---

## 📝 场景1: 新词典项 (最常见)

**示例**: 发现`AV1`视频编码未识别

### 操作

**文件**: `configs/lexicon.yaml`

```yaml
entries:
  - id: video_codec_av1
    tokens: ["AV1", "av1"]
    emit:
      - type: video_codec
        normalized: "AV1"
    confidence: 0.95
    examples:
      - "[Group] Title - 01 [1080p AV1]"
```

### 测试

```bash
./cmd/parse/parse "[Group] Title - 01 [1080p AV1]"
```

**时间**: 5-10分钟 | **步骤**: 2步 | **风险**: 低

---

## 📐 场景2: 新模板格式

**示例**: 发现`Title | 01 | 1080p`格式未匹配

### 操作

**文件**: `configs/template_catalog.yaml`

```yaml
tv_episode_templates:
  - id: "TF_TV_PIPE_SEPARATOR_01"
    name: "TITLE | EP | TECH"
    content_types: ["TV_EP"]
    signature: "T E X"
    confidence: 0.85
    examples:
      - "药屋少女 | 01 | 1080p HEVC"
```

### 测试

```bash
./cmd/parse/parse "药屋少女 | 01 | 1080p HEVC"
```

**时间**: 15-30分钟 | **步骤**: 3步 | **风险**: 中

---

## 🆕 场景3: 新字段类型

**示例**: 需要添加`subtitle_format`字段

### 操作

**文件1**: `configs/attribute_config.yaml`

```yaml
fields:
  subtitle_format:
    category: technical
    type: enum
    values: [ASS, SRT, SUP, PGS]
    extraction:
      source: tech_slot
      method: dictionary_match
```

**文件2**: `configs/lexicon.yaml`

```yaml
entries:
  - id: subtitle_format_ass
    tokens: ["ASS", "ass"]
    emit:
      - type: subtitle_format
        normalized: "ASS"
```

**文件3**: `pkg/model/types.go`

```go
const (
    AttrSubtitleFormat AttrType = "subtitle_format"
)

type ParseResult struct {
    SubtitleFormat string `json:"subtitle_format,omitempty"`
}
```

### 测试

```bash
go test ./pkg/parser -v
./cmd/parse/parse "[Group] Title - 01 [1080p ASS]"
```

**时间**: 30-60分钟 | **步骤**: 5步 | **风险**: 中高

---

## 🎬 场景4: 新Content Type

**示例**: 需要添加`PV` (Preview)类型

### 操作

**文件1**: `configs/inference_rules.yaml`

```yaml
content_type_inference:
  - name: "pv_by_keyword"
    priority: 92
    condition:
      has_pv_keyword: true
    result: "PV"
    keywords: ["PV", "Preview", "预告"]
```

**文件2**: `configs/template_catalog.yaml`

```yaml
pv_templates:
  - id: "TF_PV_STANDARD_01"
    name: "[GROUP] TITLE PV [TECH]"
    content_types: ["PV"]
```

**文件3**: `configs/attribute_config.yaml`

```yaml
fields:
  content_kind:
    values:
      - PV  # 新增
```

**文件4**: `pkg/model/types.go`

```go
const ContentKindPV = "PV"
```

### 测试

```bash
go test ./pkg/inference -v
./cmd/parse/parse "[Group] 药屋少女 PV [1080p]"
```

**时间**: 60-90分钟 | **步骤**: 5步 | **风险**: 高

---

## 🚧 场景5: 新边界规则

**示例**: Year应该是强边界

### 操作

**文件**: `configs/boundary_rules.yaml`

```yaml
strong_boundaries:
  - type: "year"
    priority: 75
    reason: "Year是强边界,不应混入TITLE"
    examples:
      - "[Group] Title 2024 - 01 [1080p]"
```

### 测试

```bash
go test ./pkg/boundary -v
./cmd/parse/parse "[Group] Title 2024 - 01 [1080p]"
```

**时间**: 10-20分钟 | **步骤**: 2步 | **风险**: 中

---

## 🔍 验证清单

添加规则后,检查:

- [ ] 配置文件语法正确 (YAML格式)
- [ ] 添加了examples
- [ ] 运行了单元测试
- [ ] 使用命令行工具测试了实际case
- [ ] 更新了version和updated_at
- [ ] 记录了变更 (可选)

---

## 📊 v1 vs v2 对比

### 添加新词典项

| 架构 | 步骤 | 文件数 | 时间 |
|------|------|--------|------|
| **v1** | 5步 | 5个文件 | 30-60分钟 |
| **v2** | 2步 | 1个文件 | 5-10分钟 |
| **改进** | ⬇️ 60% | ⬇️ 80% | ⬇️ 80% |

---

## 🛠️ 常用命令

```bash
# 搜索现有规则
grep -r "AV1" title-parser/configs/

# 测试解析
./cmd/parse/parse "测试标题"

# 运行测试
go test ./pkg/config -v
go test ./pkg/inference -v
go test ./pkg/boundary -v

# 查看日志
tail -f logs/parser.log
```

---

## 📚 详细文档

完整指南请参考: [ADDING_NEW_RULES_GUIDE.md](./docs/ADDING_NEW_RULES_GUIDE.md)

包含:
- 详细的操作步骤
- 完整的代码示例
- 最佳实践
- 常见问题解答

