// 网关 id 的显示 ↔ 数据映射：面板输入框只显示短名（supplier/model），
// 草稿与发布载荷始终保持全限定 id（supplier/channel/model）。
// 显示层剥离 channel 曾直接改写数据 id，导致三段式与历史短 id 撞车、
// 发布又把短 id 写回网关；此后剥离只发生在渲染，写回必经 resolveGatewayId。

// 短显示：三段式去 channel 段，其余形态原样。
export function shortGatewayId(id: string): string {
  const parts = id.split("/");
  return parts.length === 3 ? `${parts[0]}/${parts[2]}` : id;
}

// 输入框文本映射回全限定 id：
// - 与当前全 id 的短显示一致 → 视为未实质修改，保留原全 id；
// - 恰好是已知全 id 之一（如粘贴）→ 采用该全 id；
// - 其余（自定义/历史短 id）→ 原样保留，后端本就兼容短 id。
export function resolveGatewayId(
  typed: string,
  currentFull: string,
  knownFullIds: Iterable<string>,
): string {
  if (typed === shortGatewayId(currentFull)) return currentFull;
  for (const known of knownFullIds) {
    if (known === typed) return known;
  }
  return typed;
}
