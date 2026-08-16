// Package build 存放构建期注入的版本信息（OTA 升级与安装脚本共用）。
package build

// Version 当前版本号。发布时用 -ldflags "-X .../build.Version=x.y.z" 覆盖。
var Version = "1.0.1"
