# oci-sync Feature Specification

## 目标

`oci-sync` 是一个专注于 OCI 协议的本地/远程目录镜像同步工具。

- 支持 `oci://` 协议（远程 Registry）
```
oci://image-name
```
- 支持 `file://` 协议（本地 OCI Layout）
```
file://<path-dir>:<image-name>
```


## 功能

1. 推送目录
```
oci-sync push --dir /to/path/dir oci://<image-name>
oci-sync push --dir /to/path/dir file://<path-dir>:<image-name>
```
2. 拉取目录
```
oci-sync pull oci://<image-name> --dir /to/path/dir
oci-sync push file://<path-dir>:<image-name> --dir /to/path/dir
```

