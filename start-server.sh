#!/bin/bash

# Title Parser HTTP Server 启动脚本

# 设置环境变量
export PORT="${PORT:-8080}"
export GIN_MODE="${GIN_MODE:-release}"
export CONFIG_DIR="${CONFIG_DIR:-./configs}"

# 检查二进制文件是否存在
if [ ! -f "./bin/parser-server" ]; then
    echo "错误: 找不到 parser-server 二进制文件"
    echo "请先运行: go build -o bin/parser-server cmd/server/main.go"
    exit 1
fi

# 检查配置目录是否存在
if [ ! -d "$CONFIG_DIR" ]; then
    echo "错误: 配置目录不存在: $CONFIG_DIR"
    exit 1
fi

echo "启动 Title Parser Service..."
echo "端口: $PORT"
echo "模式: $GIN_MODE"
echo "配置目录: $CONFIG_DIR"
echo ""

# 启动服务器
./bin/parser-server
