# Gateway-Sep WebUI

Status: ready-for-agent

## 背景
供应商与网关存储已解耦，网关仅在显式发布时构建。前端需移除 Profile/Models 保存时的隐式 preview/apply，改为本地保存 + 提示到网关发布，网关面板集中展示 Current vs Proposed 并提供“应用到 Pi”入口。

## 目标
- ProfilesPanel ProfileForm/ModelsModal：移除 previewGateway/applyGateway 与 GatewayPreviewModal，仅调用 addProfile/updateProfile/updateModels/expose，toast“已保存到本地，需到网关发布”+refresh
- GatewayPanel：顶部 Current vs Proposed 状态条（added/removed/changed、待发布数、上次发布时间），主按钮“应用到 Pi”调 PUT /models/gateway，成功 refresh+更新发布时间，失败保留 config；首次进入 diff 非空时顶部提示“检测到本地与 Pi 网关不一致，是否立即同步”，默认不自动写
- 保持 piModel 派生规则（provider_id_for/enrich/prefix）不变，仅触发时机改为发布时
- 补充 vitest 覆盖

## 非目标
- 不改动 piModel 前端派生逻辑与后端 enrich 规则
- 不接管非网关 provider 编辑

## 验收
- Profile 保存后不触发 preview/apply，不弹 modal，toast 提示且 refresh
- Models 保存后同上
- Gateway 顶部状态条 diff/待发布数正确， mismatch 横幅首次仅提示不自动写
- “应用到 Pi”成功 refresh 且 pending 清零，失败保留编辑态
- npm --prefix webui typecheck / NODE_ENV=test vitest 全绿
