# Bifrost 监控指南

## 指标采集
- 请求吞吐量
- 延迟 P99
- 错误率

## 告警规则
- 错误率 > 1% 持续 5min
- P99 延迟 > 500ms 持续 5min

## Grafana Dashboard
导入 `dashboards/bifrost.json`
