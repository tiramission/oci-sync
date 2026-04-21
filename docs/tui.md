# OCI-Sync TUI 实现文档

## 概述

OCI-Sync 现已支持全屏交互式 TUI（终端用户界面），提供比命令行更友好的交互体验，特别适合频繁操作 artifacts 的场景。

## 启用方式

TUI 是实验性功能，需要在配置文件中启用：

```yaml
experiments:
  tui: true
```

启用后，运行 `oci-sync tui` 即可启动交互界面。

## 架构设计

### 核心组件

1. **Model** (`model.go`, 611 行)
   - 主 TUI 模型，管理全局状态
   - 使用 Bubble Tea 框架实现事件驱动架构
   - 支持多个屏幕（Shortcuts、Artifacts、Detail）
   - 处理键盘输入和消息分发

2. **Modal** (`modal.go`, 397 行)
   - 模态对话框系统
   - 支持上传、下载、删除等多种模态
   - 多步骤表单输入（分步收集数据）
   - 密码输入自动屏蔽

3. **Operations** (`operations.go`, 140 行)
   - OCI 操作包装器
   - `PushArtifact`：上传 artifacts（含加密）
   - `PullArtifact`：下载 artifacts（含解密）
   - `DeleteArtifact`：删除 artifacts

4. **Run** (`run.go`, 15 行)
   - 应用启动入口
   - 初始化 Bubble Tea 程序

### 数据流

```
用户输入
    ↓
KeyMsg 处理 (Update)
    ↓
判断是否在模态中 → 模态处理 → 执行操作
           ↓
      屏幕处理 → 状态更新
           ↓
渲染输出 (View)
    ↓
显示在终端
```

### 屏幕架构

#### 1. Shortcuts 屏幕
- 显示所有配置的 shortcuts
- 选择并进入特定 shortcut 的 artifacts 列表
- 导航：↑/↓，Enter 进入

#### 2. Artifacts 屏幕
- 显示选中 shortcut 下的所有 artifacts
- 支持快速操作：u(上传), d(下载), x(删除), r(刷新)
- 显示加密状态（🔒）和标签

#### 3. Detail 屏幕
- 展示单个 artifact 的详细信息
- 包括：Tag、Digest、Repository、加密状态、版本、标签等

## 关键特性

### 1. 模态对话框系统

**上传模态** (`ModalUpload`)
```
Step 0: 输入本地路径
  ↓
Step 1: 输入 tag
  ↓
Step 2: 输入密码（可选）
  ↓
执行上传
```

**下载模态** (`ModalDownload`)
```
Step 0: 输入下载路径
  ↓
Step 1: 输入密码（如需要）
  ↓
执行下载
```

**删除模态** (`ModalDelete`)
```
确认删除?
  ↓
y: 确认 / n: 取消
```

### 2. 密码安全处理

- 密码字段输入自动屏蔽显示（`•`）
- 使用 `huh.EchoModePassword` 实现
- 支持多步骤表单中的单个密码字段

### 3. 错误处理

- 输入验证（路径、tag 等）
- 错误消息实时显示
- 成功操作提示（✓）

### 4. 自动刷新

- 上传/删除后自动重新加载 artifacts 列表
- 用户无需手动刷新

## 技术栈

### 依赖库

| 库 | 版本 | 用途 |
|----|------|------|
| `github.com/charmbracelet/bubbletea` | v1.3.10 | TUI 框架 |
| `github.com/charmbracelet/lipgloss` | v1.1.0 | 样式和布局 |
| `github.com/charmbracelet/huh` | v1.0.0 | 表单输入（弃用，改用 bubbletea） |

### 编码模式

1. **事件驱动**：所有交互通过消息传递
2. **函数式**：纯函数处理状态更新
3. **分层**：Model、View、Update 清晰分离

## 使用示例

### 启动 TUI

```bash
oci-sync tui
```

### 工作流程

1. 启动应用 → 显示 Shortcuts 列表
2. 选择 shortcut → 显示该 shortcut 的 artifacts
3. 选择 artifact 操作：
   - `Enter` 查看详情
   - `u` 上传新 artifact
   - `d` 下载 artifact
   - `x` 删除 artifact
   - `r` 刷新列表

### 快捷键参考

| 键 | 功能 | 范围 |
|----|------|------|
| ↑/↓, k/j | 导航 | 列表 |
| Enter | 选择/提交 | 所有 |
| u | 上传 | Artifacts |
| d | 下载 | Artifacts |
| x | 删除 | Artifacts |
| b | 返回 | Artifacts/Detail |
| r | 刷新 | Artifacts |
| Esc | 取消 | 模态 |
| y/n | 确认/拒绝 | 确认对话框 |
| q | 退出 | 全局 |
| Ctrl+C | 强制退出 | 全局 |

## 文件结构

```
internal/tui/
├── model.go           # 主模型 (611 行)
├── model_test.go      # 单元测试 (57 行)
├── modal.go           # 模态系统 (397 行)
├── operations.go      # OCI 操作 (140 行)
└── run.go             # 启动器 (15 行)

cmd/
└── tui.go            # CLI 集成 (27 行)
```

## 测试覆盖

| 测试 | 用途 |
|------|------|
| `TestNewModel` | 模型初始化 |
| `TestNewUploadModal` | 上传模态创建 |
| `TestNewDownloadModal` | 下载模态创建 |
| `TestNewDeleteModal` | 删除模态创建 |
| `TestLoadShortcutsInit` | 配置加载 |

## 扩展方向

1. **标签编辑** - 支持在 TUI 中修改 artifacts 的标签
2. **进度条** - 显示上传/下载进度
3. **批量操作** - 支持多选操作
4. **搜索过滤** - 按 tag 或标签搜索 artifacts
5. **配置管理** - 在 TUI 中管理 shortcuts

## 性能特性

- **懒加载**：仅在选择 shortcut 时加载 artifacts
- **异步操作**：长时间操作（上传/下载）不阻塞 UI
- **流式渲染**：只重新渲染变化的部分

## 已知限制

1. 标签编辑功能待实现
2. 不支持批量操作
3. 不支持搜索/过滤
4. 不支持进度条显示
