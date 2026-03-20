# oci-sync

将本地文件或目录同步到 OCI 兼容的镜像仓库中。支持文件/目录、可选加密，使用 Docker credential store 进行认证。

## 安装

### 使用 Go
```bash
go install github.com/tiramission/oci-sync@latest
```

或从源码构建：
```bash
git clone https://github.com/tiramission/oci-sync.git
cd oci-sync
go build -o oci-sync .
```

### 使用 Nix (Flake)
本项目已提供 `flake.nix`，Nix 用户可直接运行：
```bash
nix run github:tiramission/oci-sync -- --help
```

开发者也可直接进入开发环境：
```bash
nix develop github:tiramission/oci-sync
```

## 前置条件

已通过 `docker login` 登录目标仓库：

```bash
docker login registry.example.com
```

## 使用

### push — 推送到仓库

```bash
# 推送目录（不加密）
oci-sync push ./mydir registry.example.com/myrepo:latest

# 推送文件（加密）
oci-sync push ./secret.txt registry.example.com/myrepo:encrypted --passphrase mypassword
```

### pull — 从仓库拉取

```bash
# 拉取（不加密）
oci-sync pull registry.example.com/myrepo:latest ./output

# 拉取并解密
oci-sync pull registry.example.com/myrepo:encrypted ./output --passphrase mypassword
```

### delete — 删除仓库中的文件

```bash
# 从远程仓库删除推送的文件
oci-sync delete registry.example.com/myrepo:latest
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `local_path` | 本地文件或目录路径 |
| `remote_path` | OCI 仓库引用，格式：`<registry>/<repository>:<tag>` |
| `--passphrase` | 加密/解密口令（可选） |

## 工作原理

**push**：本地文件/目录 → tar.gz 打包 → [可选] AES-256-GCM 加密 → 推送至 OCI 仓库

**pull**：从 OCI 仓库拉取 → [可选] 解密 → 解压 tar.gz → 写入本地

加密使用 scrypt 从口令派生密钥（N=32768），每次加密使用随机 salt 和 nonce，安全可靠。

认证直接读取 `~/.docker/config.json`，与 Docker credential store 完全兼容，支持 macOS Keychain、Windows Credential Manager 和 Linux secret service。

## 详细设计

见 [docs/design.md](docs/design.md)。
