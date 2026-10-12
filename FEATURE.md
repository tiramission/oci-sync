# 项目介绍

`oci-sync` 命令行工具，用于将本地文件同步到 OCI 兼容的镜像仓库中，支持单文件和目录，支持加密
使用 go 语言实现

## 主要功能

- `oci-sync push`：将本地文件同步到 OCI 兼容的镜像仓库中。
- `oci-sync pull`：从 OCI 兼容的镜像仓库中同步文件到本地。
- `oci-sync delete`：从 OCI 兼容的镜像仓库中删除文件(镜像)。
- `oci-sync list`：列出 OCI 仓库中的镜像记录，支持标签筛选。
- `oci-sync label set`：为已存在的镜像设置标签。
- `oci-sync label unset`：删除镜像上的标签。
- `oci-sync alias list`：列出所有配置的 shortcuts。
- `oci-sync alias add`：添加新的 shortcut。
- `oci-sync alias remove`：删除 shortcut。
- `oci-sync recent`：查看活动历史记录。
- `oci-sync tui`：启动全屏分栏交互式 TUI 管理 shortcuts 的 artifacts。
- `oci-sync gui`：启动原生桌面 GUI（中文界面）浏览仓库，推送、拉取、删除 artifact 并编辑标签。
- `oci-sync <name> push`：快捷推送命令，通过 `shortcuts.<name>.repo` 配置仓库，仅用 `--tag` 指定远程标签，支持 `--label`。
- `oci-sync <name> pull`：快捷拉取命令，通过 `shortcuts.<name>.repo` 配置仓库，仅用 `--tag` 指定远程标签。
- `oci-sync <name> list`：快捷列举命令，通过 `shortcuts.<name>.repo` 配置仓库，直接列出所有 tags，支持 `--label` 筛选。
- `oci-sync <name> delete`：快捷删除命令，通过 `shortcuts.<name>.repo` 配置仓库，仅用 `--tag` 指定删除目标。


### 认证
- 支持配置文件 `auths.<registry>.username/password` 为每个仓库配置独立凭据
- 回退到 Docker credential store（`~/.docker/config.json`），支持 macOS, Windows, Linux


### 实现

尽量不创建轮子，使用现有的库。实现过程：对于文件打包加密压缩，然后创建 oci image，最后推送到镜像仓库，拉取过程相反。

各命令的用法、参数与示例见 [README.md](README.md)；TUI / GUI 的交互设计与实现约束（键位、布局、异步与取消、可读性等）见 [docs/design.md](docs/design.md) 第 7 节。

### 配置文件

配置文件使用 YAML 格式，搜索路径如下：

1. 当前工作目录 `./oci-sync.yaml`
2. 用户配置目录 `~/.config/oci-sync/oci-sync.yaml`

配置文件格式：

```yaml
shortcuts:
  x:
    repo: registry.example.com/myteam/files

auths:
  registry.example.com:
    username: myuser
    password: mytoken
  another.registry.com:
    username: user2
    password: token2
```

认证优先级：**配置文件 `auths` > Docker credential store**

## Nix 集成

### Home Manager 模块

本项目提供 home-manager 模块，支持通过 Nix 配置管理 oci-sync：

```nix
{
  programs.oci-sync = {
    enable = true;
    settings = {
      shortcuts = {
        x.repo = "registry.example.com/myteam/files";
      };
    };
  };
}
```

可用选项：

| 选项 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enable` | bool | false | 启用 oci-sync |
| `package` | package | - | 自定义包版本 |
| `settings.shortcuts.<name>.repo` | string | "" | 快捷命令默认仓库 |
