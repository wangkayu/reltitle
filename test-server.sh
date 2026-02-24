#!/bin/bash

# Title Parser HTTP Server 测试脚本

BASE_URL="http://localhost:8080"

echo "=========================================="
echo "Title Parser HTTP Server 测试"
echo "=========================================="
echo ""

# 颜色定义
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# 测试计数
TOTAL=0
PASSED=0
FAILED=0

# 测试函数
test_endpoint() {
    local name=$1
    local method=$2
    local endpoint=$3
    local data=$4
    
    TOTAL=$((TOTAL + 1))
    echo "测试 $TOTAL: $name"
    
    if [ "$method" = "GET" ]; then
        response=$(curl -s -w "\n%{http_code}" "$BASE_URL$endpoint")
    else
        response=$(curl -s -w "\n%{http_code}" -X "$method" "$BASE_URL$endpoint" \
            -H "Content-Type: application/json" \
            -d "$data")
    fi
    
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | sed '$d')
    
    if [ "$http_code" = "200" ]; then
        echo -e "${GREEN}✓ 通过${NC} (HTTP $http_code)"
        echo "$body" | jq . 2>/dev/null || echo "$body"
        PASSED=$((PASSED + 1))
    else
        echo -e "${RED}✗ 失败${NC} (HTTP $http_code)"
        echo "$body"
        FAILED=$((FAILED + 1))
    fi
    echo ""
}

# 检查服务器是否运行
echo "检查服务器状态..."
if ! curl -s "$BASE_URL/health" > /dev/null 2>&1; then
    echo -e "${RED}错误: 服务器未运行${NC}"
    echo "请先启动服务器: ./start-server.sh"
    exit 1
fi
echo -e "${GREEN}服务器正在运行${NC}"
echo ""

# 测试 1: 健康检查
test_endpoint "健康检查" "GET" "/health" ""

# 测试 2: 单个解析 - 标准 TV 剧集
test_endpoint "单个解析 - 标准 TV 剧集" "POST" "/api/v1/parse" \
'{
  "title": "[Lilith-Raws] 葬送的芙莉莲 / Sousou no Frieren - 01 [Baha][WEB-DL][1080p][AVC AAC][CHT][MP4]"
}'

# 测试 3: 单个解析 - 带季度信息
test_endpoint "单个解析 - 带季度信息" "POST" "/api/v1/parse" \
'{
  "title": "[ANi] 间谍过家家 第二季 - 01 [1080P][Baha][WEB-DL][AAC AVC][CHT][MP4]"
}'

# 测试 4: 单个解析 - 剧场版
test_endpoint "单个解析 - 剧场版" "POST" "/api/v1/parse" \
'{
  "title": "[Nekomoe kissaten] 剧场版 紫罗兰永恒花园 Violet Evergarden the Movie [BDRip 1080p HEVC-10bit FLAC]"
}'

# 测试 5: 批量解析
test_endpoint "批量解析" "POST" "/api/v1/parse/batch" \
'{
  "titles": [
    "[Lilith-Raws] 葬送的芙莉莲 / Sousou no Frieren - 01 [Baha][WEB-DL][1080p][AVC AAC][CHT][MP4]",
    "[ANi] 间谍过家家 第二季 - 01 [1080P][Baha][WEB-DL][AAC AVC][CHT][MP4]",
    "[Nekomoe kissaten] 剧场版 紫罗兰永恒花园 Violet Evergarden the Movie [BDRip 1080p HEVC-10bit FLAC]"
  ]
}'

# 测试 6: 错误处理 - 空标题（预期返回 400）
echo "测试 $((TOTAL + 1)): 错误处理 - 空标题（预期返回 400）"
TOTAL=$((TOTAL + 1))
response=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/v1/parse" \
    -H "Content-Type: application/json" \
    -d '{"title": ""}')
http_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | sed '$d')

if [ "$http_code" = "400" ]; then
    echo -e "${GREEN}✓ 通过${NC} (HTTP $http_code - 正确返回错误)"
    echo "$body" | jq . 2>/dev/null || echo "$body"
    PASSED=$((PASSED + 1))
else
    echo -e "${RED}✗ 失败${NC} (HTTP $http_code - 应该返回 400)"
    echo "$body"
    FAILED=$((FAILED + 1))
fi
echo ""

# 总结
echo "=========================================="
echo "测试总结"
echo "=========================================="
echo "总计: $TOTAL"
echo -e "通过: ${GREEN}$PASSED${NC}"
echo -e "失败: ${RED}$FAILED${NC}"
echo ""

if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}所有测试通过！${NC}"
    exit 0
else
    echo -e "${RED}部分测试失败${NC}"
    exit 1
fi
