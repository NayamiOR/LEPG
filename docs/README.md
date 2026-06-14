---
title: LEPG 文档索引
description: LEPG 文档库入口（MOC）——按 设计 / 配置 / 报告 / 规划 分类导航全部 Foam 笔记
type: index
tags: [index, moc]
---

# LEPG 文档索引

LEPG（Lightweight Edge Piercing Gateway）文档库。本笔记是入口与地图（MOC），所有笔记通过 `[[wikilink]]` 互联，可在 Foam 知识图谱中查看关联。

## 设计与协议

- [[message-protocol|消息协议]] — TLV 帧结构与七种消息类型
- [[mqtt-broker-design|MQTT Broker 设计]] — Topic、数据桥接、认证、ACL、QoS
- [[authentication|网关认证逻辑]] — 握手认证从静态 SN/Token 到一次性 Token 的演进

## 配置参考

> 入口笔记：[[overview|配置系统总览]]（Provider Chain 与配置来源优先级）

- [[overview|配置系统总览]]
- [[server-config|服务器配置 (lepgs)]]
- [[client-config|客户端配置 (lepgc)]]
- [[config-examples|完整配置示例]]
- [[cli-and-env|CLI 命令与环境变量参考]]
- [[sensitive-fields|敏感字段处理]]
- [[config-migration|迁移指南（旧系统 → 新系统）]]
- [[config-dev-guide|开发者指南：添加新配置项]]
- [[config-internals|配置系统内部机制]]
- [[modbus-config|Modbus 设备配置]]

## 分析报告与案例

- [[handshake-heartbeat-analysis|握手与心跳逻辑分析报告]]
- [[device-state-analysis|设备状态记录实现状态分析报告]]
- [[config-refactor-report|配置系统重构案例报告]]

## 规划

- [[roadmap|LEPG 项目现状报告 & 开发路线图]]

## 图表资源

- [assets/data-link.excalidraw](./assets/data-link.excalidraw) — 数据链路图（Excalidraw）

---

## 主题导航

按关注点快速定位：

- **协议与握手**：[[message-protocol]] · [[authentication]] · [[handshake-heartbeat-analysis]]
- **MQTT 与数据出口**：[[mqtt-broker-design]] · [[device-state-analysis]]
- **设备状态与存活**：[[device-state-analysis]] · [[modbus-config]]
- **配置系统**：[[overview]] · [[config-internals]] · [[config-refactor-report]] · [[config-migration]]
- **下一步做什么**：[[roadmap]]
