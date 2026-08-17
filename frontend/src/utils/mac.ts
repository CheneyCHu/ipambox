// isRandomMAC 判断是否为随机/本地管理 MAC（LAA 位为 1）。
// iOS/Android 的「私有无线局域网地址」功能会生成此类 MAC，
// 它们不属于任何厂商，OUI 查询必然无结果。
export function isRandomMAC(mac: string | undefined): boolean {
  if (!mac) return false
  const hex = parseInt(mac.slice(0, 2), 16)
  return !isNaN(hex) && (hex & 0x02) !== 0
}
