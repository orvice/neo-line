# ADR-0002：active/previous 单文档双版本模型

## 状态

已接受（2026-05）

## 背景

ManagedCertificate 需要支持回滚与续期，同时限制本地私钥累积。MongoDB 首版部署 **不要求** replica set，不能依赖多文档事务作为运行前提。

## 决策

- 每个 ManagedCertificate 在 **单个** `managed_certificates` 文档内嵌套 `active_version` 与 `previous_version` 两个完整 **CertificateVersion**（含 PEM）。
- 新版本激活时：原 active → previous；更老 previous 的 PEM **本地删除**，不向 CA 隐式吊销。
- Admin 可手动激活未吊销 previous（含已过期）；Server **永远** 只获取 active。
- 版本切换与 CAS 使用 **单文档** 原子更新（`ReplaceOne` / 条件字段），不拆独立 `certificate_versions` collection。

## 后果

- 回滚与切换延迟低、语义清晰；历史版本不可无限追溯。
- 大文档尺寸受 PEM 大小影响；仅保留两版可控。
- 测试与 Store adapter 必须覆盖 active ID 冲突时的 compare-and-swap 失败路径。

## 参考

- GitHub #13 Solution / User Stories 41–43
- [托管证书 — desired/active 分离](./../certificate-management-managed-certificates.md)
