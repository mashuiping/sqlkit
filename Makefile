# Makefile for sqlkit cross-compilation
# 支持交叉编译到多个平台

# 项目信息
PROJECT_NAME := sqlkit
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date +%Y%m%d_%H%M%S)
LDFLAGS := -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)

# 输出目录
BUILD_DIR := build
BIN_DIR := $(BUILD_DIR)/bin

# 编译目标平台
# Mac ARM (Apple Silicon)
MAC_ARM_OS := darwin
MAC_ARM_ARCH := arm64
MAC_ARM_BINARY := $(BIN_DIR)/$(PROJECT_NAME)-darwin-arm64

# Mac Intel
MAC_INTEL_OS := darwin
MAC_INTEL_ARCH := amd64
MAC_INTEL_BINARY := $(BIN_DIR)/$(PROJECT_NAME)-darwin-amd64

# Windows x86_64
WIN_OS := windows
WIN_ARCH := amd64
WIN_BINARY := $(BIN_DIR)/$(PROJECT_NAME)-windows-amd64.exe

# Ubuntu/Linux amd64
LINUX_OS := linux
LINUX_ARCH := amd64
LINUX_BINARY := $(BIN_DIR)/$(PROJECT_NAME)-linux-amd64

# Ubuntu/Linux arm64
LINUX_ARM64_OS := linux
LINUX_ARM64_ARCH := arm64
LINUX_ARM64_BINARY := $(BIN_DIR)/$(PROJECT_NAME)-linux-arm64

# 默认目标
.PHONY: all
all: clean mac-arm mac-intel windows linux linux-arm64

# Mac ARM (Apple Silicon) 编译
.PHONY: mac-arm
mac-arm:
	@echo "Building for Mac ARM (Apple Silicon)..."
	@mkdir -p $(BIN_DIR)
	GOOS=$(MAC_ARM_OS) GOARCH=$(MAC_ARM_ARCH) go build -ldflags "$(LDFLAGS)" -o $(MAC_ARM_BINARY) .
	@echo "✓ Built: $(MAC_ARM_BINARY)"

# Mac Intel 编译
.PHONY: mac-intel
mac-intel:
	@echo "Building for Mac Intel..."
	@mkdir -p $(BIN_DIR)
	GOOS=$(MAC_INTEL_OS) GOARCH=$(MAC_INTEL_ARCH) go build -ldflags "$(LDFLAGS)" -o $(MAC_INTEL_BINARY) .
	@echo "✓ Built: $(MAC_INTEL_BINARY)"

# Windows x86_64 编译
.PHONY: windows
windows:
	@echo "Building for Windows x86_64..."
	@mkdir -p $(BIN_DIR)
	GOOS=$(WIN_OS) GOARCH=$(WIN_ARCH) go build -ldflags "$(LDFLAGS)" -o $(WIN_BINARY) .
	@echo "✓ Built: $(WIN_BINARY)"

# Linux/Ubuntu amd64 编译
.PHONY: linux
linux:
	@echo "Building for Linux/Ubuntu amd64..."
	@mkdir -p $(BIN_DIR)
	GOOS=$(LINUX_OS) GOARCH=$(LINUX_ARCH) go build -ldflags "$(LDFLAGS)" -o $(LINUX_BINARY) .
	@echo "✓ Built: $(LINUX_BINARY)"

# Linux/Ubuntu arm64 编译
.PHONY: linux-arm64
linux-arm64:
	@echo "Building for Linux/Ubuntu arm64..."
	@mkdir -p $(BIN_DIR)
	GOOS=$(LINUX_ARM64_OS) GOARCH=$(LINUX_ARM64_ARCH) go build -ldflags "$(LDFLAGS)" -o $(LINUX_ARM64_BINARY) .
	@echo "✓ Built: $(LINUX_ARM64_BINARY)"

# 清理编译产物
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@echo "✓ Cleaned"

# 显示帮助信息
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  make all         - Build for all platforms"
	@echo "  make mac-arm     - Build for Mac ARM (Apple Silicon)"
	@echo "  make mac-intel   - Build for Mac Intel"
	@echo "  make windows     - Build for Windows x86_64"
	@echo "  make linux       - Build for Linux/Ubuntu amd64"
	@echo "  make linux-arm64 - Build for Linux/Ubuntu arm64"
	@echo "  make clean       - Clean build artifacts"
	@echo "  make help        - Show this help message"
