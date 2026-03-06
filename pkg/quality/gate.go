package quality

import (
	"strings"

	"title-parser/pkg/model"
)

// QualityCheck 质量检查结果
type QualityCheck struct {
	Status    string   // PASS, FAIL
	Score     float64  // 0.0 - 1.0
	RiskLevel string   // LOW, MEDIUM, HIGH
	Reasons   []string // 失败原因
}

// CheckTitleQuality 检查 title 解析质量
func CheckTitleQuality(res *model.ParseResult) *QualityCheck {
	check := &QualityCheck{
		Status:    "PASS",
		Score:     1.0,
		RiskLevel: "LOW",
		Reasons:   []string{},
	}

	if res == nil {
		check.Status = "FAIL"
		check.Score = 0.0
		check.RiskLevel = "HIGH"
		check.Reasons = append(check.Reasons, "nil_result")
		return check
	}
	title := res.CanonicalTitle()
	episode := res.CanonicalEpisode()
	episodeRange := res.CanonicalEpisodeRange()
	packKind := res.CanonicalPackKind()

	// 检查 1: Title 是否为空
	if strings.TrimSpace(title) == "" {
		check.Status = "FAIL"
		check.Score -= 0.5
		check.RiskLevel = "HIGH"
		check.Reasons = append(check.Reasons, "empty_title")
	}

	// 检查 2: Title 是否过短 (< 2 字符)
	if len([]rune(strings.TrimSpace(title))) < 2 {
		check.Status = "FAIL"
		check.Score -= 0.3
		check.RiskLevel = "HIGH"
		check.Reasons = append(check.Reasons, "title_too_short")
	}

	// 检查 3: Title 是否只包含技术标签
	if isTitleOnlyTechTags(title) {
		check.Status = "FAIL"
		check.Score -= 0.4
		check.RiskLevel = "HIGH"
		check.Reasons = append(check.Reasons, "title_only_tech_tags")
	}

	// 检查 4: Title 是否包含明显的技术属性
	if titleContainsTechAttributes(title, res.Attributes) {
		check.Score -= 0.2
		check.RiskLevel = "MEDIUM"
		check.Reasons = append(check.Reasons, "title_contains_tech_attrs")
	}

	// 检查 5: Title 边界是否可疑
	if isTitleBoundarySuspicious(res) {
		check.Score -= 0.3
		check.RiskLevel = "MEDIUM"
		check.Reasons = append(check.Reasons, "suspicious_title_boundary")
	}

	// 检查 6: 是否缺少关键字段
	if isMissingCriticalFields(res.ContentKind, episode, episodeRange, packKind) {
		check.Score -= 0.1
		check.RiskLevel = "MEDIUM"
		check.Reasons = append(check.Reasons, "missing_critical_fields")
	}

	// 检查 7: Template 置信度过低
	if res.TemplateConfidence > 0 && res.TemplateConfidence < 0.5 {
		check.Score -= 0.1
		check.RiskLevel = "MEDIUM"
		check.Reasons = append(check.Reasons, "low_template_confidence")
	}

	// 最终判断
	if check.Score < 0.0 {
		check.Score = 0.0
	}

	if check.Score < 0.6 {
		check.Status = "FAIL"
		if check.RiskLevel == "LOW" {
			check.RiskLevel = "MEDIUM"
		}
	}

	return check
}

// isTitleOnlyTechTags 检查 title 是否只包含技术标签
func isTitleOnlyTechTags(title string) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return true
	}

	// 常见技术标签和噪声词
	techTags := []string{
		"1080p", "720p", "480p", "2160p",
		"x264", "x265", "h264", "h265", "hevc", "avc",
		"aac", "flac", "opus", "dts",
		"bluray", "webrip", "webdl", "bdrip",
		"ma10p", "ma8p",
		"crc", "crc32", // CRC 校验码
	}

	titleLower := strings.ToLower(title)

	// 检查是否以 CRC 开头(常见错误)
	if strings.HasPrefix(titleLower, "crc") {
		return true
	}

	for _, tag := range techTags {
		if strings.Contains(titleLower, tag) {
			// 如果 title 主要由技术标签组成
			if len(title) < len(tag)+5 {
				return true
			}
		}
	}

	return false
}

// titleContainsTechAttributes 检查 title 是否包含技术属性
func titleContainsTechAttributes(title string, attrs []model.Attribute) bool {
	if title == "" {
		return false
	}

	titleLower := strings.ToLower(title)

	// 检查是否包含分辨率
	resolutions := []string{"1080p", "720p", "480p", "2160p", "4k"}
	for _, res := range resolutions {
		if strings.Contains(titleLower, res) {
			return true
		}
	}

	// 检查是否包含编码格式
	codecs := []string{"x264", "x265", "h264", "h265", "hevc"}
	for _, codec := range codecs {
		if strings.Contains(titleLower, codec) {
			return true
		}
	}

	return false
}

// isTitleBoundarySuspicious 检查 title 边界是否可疑
func isTitleBoundarySuspicious(res *model.ParseResult) bool {
	if res == nil {
		return false
	}
	// 如果 title 包含方括号或圆括号,可能边界不准确
	title := res.CanonicalTitle()
	if strings.Contains(title, "[") || strings.Contains(title, "]") {
		return true
	}
	if strings.Contains(title, "(") || strings.Contains(title, ")") {
		return true
	}

	// 如果 title 以数字结尾,可能包含了 episode
	title = strings.TrimSpace(title)
	if len(title) > 0 {
		lastChar := title[len(title)-1]
		if lastChar >= '0' && lastChar <= '9' {
			// 但如果有明确的 episode 字段,则不算可疑
			if strings.TrimSpace(res.CanonicalEpisode()) == "" && strings.TrimSpace(res.CanonicalEpisodeRange()) == "" {
				return true
			}
		}
	}

	return false
}

// isMissingCriticalFields 检查是否缺少关键字段
func isMissingCriticalFields(contentKind string, episode string, episodeRange string, packKind string) bool {
	// 对于 TV 剧集,应该有 episode 或 episode_range
	if contentKind == "tv" || contentKind == "series" {
		if strings.TrimSpace(episode) == "" && strings.TrimSpace(episodeRange) == "" {
			// 除非是 pack
			if packKind == "" {
				return true
			}
		}
	}

	return false
}

// ShouldTriggerXLMR 判断是否应该触发 XLM-R 推理
func ShouldTriggerXLMR(check *QualityCheck) bool {
	if check == nil {
		return false
	}

	// 如果质量检查失败,触发 XLM-R
	if check.Status == "FAIL" {
		return true
	}

	// 如果风险等级为 HIGH,触发 XLM-R
	if check.RiskLevel == "HIGH" {
		return true
	}

	// 如果分数过低,触发 XLM-R
	if check.Score < 0.7 {
		return true
	}

	return false
}

// ExtractTokenTexts 从 ParseResult 提取 token 文本
func ExtractTokenTexts(res *model.ParseResult) []string {
	if res == nil || len(res.Tokens) == 0 {
		return []string{}
	}

	texts := make([]string, 0, len(res.Tokens))
	for _, tok := range res.Tokens {
		if !tok.Removed && strings.TrimSpace(tok.Text) != "" {
			texts = append(texts, tok.Text)
		}
	}

	return texts
}

// InferTitleSpanFromLabels 从 XLM-R 标签推断 title span
func InferTitleSpanFromLabels(tokens []model.Token, labels []string) (int, int) {
	if len(tokens) == 0 || len(labels) == 0 {
		return -1, -1
	}

	// 确保长度一致
	minLen := len(tokens)
	if len(labels) < minLen {
		minLen = len(labels)
	}

	// 找到第一个 B-TITLE
	titleStart := -1
	titleEnd := -1

	for i := 0; i < minLen; i++ {
		if tokens[i].Removed {
			continue
		}

		label := labels[i]
		if label == "B-TITLE" {
			if titleStart == -1 {
				titleStart = i
			}
			titleEnd = i + 1
		} else if label == "I-TITLE" {
			if titleStart >= 0 {
				titleEnd = i + 1
			}
		} else {
			// 遇到非 TITLE 标签,如果已经有 title,则结束
			if titleStart >= 0 {
				break
			}
		}
	}

	if titleStart >= 0 && titleEnd > titleStart {
		return titleStart, titleEnd
	}

	return -1, -1
}

// ExtractTitleFromSpan 从 token span 提取 title 文本
func ExtractTitleFromSpan(tokens []model.Token, start, end int) string {
	if start < 0 || end <= start || end > len(tokens) {
		return ""
	}

	parts := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		if !tokens[i].Removed {
			text := strings.TrimSpace(tokens[i].Text)
			if text != "" {
				parts = append(parts, text)
			}
		}
	}

	return strings.Join(parts, " ")
}
