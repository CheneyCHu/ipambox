# IPAMBox 发布与 OTA 升级说明

## 一键安装（用户侧）

```bash
# 在线安装（把 OWNER/REPO 换成你的仓库）
curl -fsSL https://raw.githubusercontent.com/OWNER/REPO/main/scripts/install.sh | bash

# 或指定二进制来源
IPAMBOX_DOWNLOAD_BASE=https://github.com/OWNER/REPO/releases/latest/download \
  bash scripts/install.sh
```

- Linux（root）：自动安装到 `/opt/ipambox` 并注册 systemd 服务
- macOS：安装到 `~/ipambox` 并注册 launchd 服务
- 低成本硬件（树莓派/ARM 盒子）：选 linux_arm64 包即可

## 发布新版本（维护者侧）

```bash
git tag v1.0.1 && git push origin v1.0.1
```

GitHub Actions 自动完成：
1. 构建前端并嵌入
2. 四平台编译（linux/darwin × amd64/arm64）
3. 创建 GitHub Release 并上传二进制
4. 生成含各平台 sha256 的 `release/latest.json` 并提交回 main

## 设备端 OTA

- 默认检查地址：`https://raw.githubusercontent.com/OWNER/REPO/main/release/latest.json`
  （部署后请在「设置」页把 `update_manifest_url` 改为你的仓库地址，或直接改代码默认值）
- 设备按自身平台（GOOS/GOARCH）自动选择安装包，SHA256 校验通过后才替换
- 替换原子化，旧版本备份为 `.bak`，失败自动回滚；升级后自动重启
- 无外网环境：把 `update_manifest_url` 指向内网任意 HTTP 服务器上的清单文件即可

## 清单格式

```json
{
  "version": "1.0.1",
  "notes": "更新说明",
  "platforms": {
    "linux_arm64":  {"url": "https://…/ipambox_linux_arm64",  "sha256": "…"},
    "linux_amd64":  {"url": "https://…/ipambox_linux_amd64",  "sha256": "…"},
    "darwin_arm64": {"url": "https://…/ipambox_darwin_arm64", "sha256": "…"},
    "darwin_amd64": {"url": "https://…/ipambox_darwin_amd64", "sha256": "…"}
  }
}
```
