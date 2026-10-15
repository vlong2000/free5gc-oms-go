# free5GC 网管中心（Go 版）

与 free5GC 同技术栈（Go）的配套网管/监控系统：**单二进制 + go:embed 前端**，
以 free5GC 暴露的真实监控面为数据源，叠加「配置文件 ↔ 运行态」一致性审计。
仅一个第三方依赖（`gopkg.in/yaml.v3`，与 free5GC 各 NF 的 factory 同库）。

## 与 free5GC 代码的对应关系

| 功能 | 数据来源（真实接口/文件） | 源码依据 |
|---|---|---|
| 注册网元清单/服务状态 | `GET {nrf}/nnrf-nfm/v1/nf-instances` | `nrf/internal/sbi/api_nfmanagement.go`；与官方 webconsole `UpdateNfProfiles()` 同源 |
| 网元健康 | 各 NF 服务前缀 `GET /`（gin 404 也视为存活）+ TCP 探针 | `*/internal/sbi/server.go` |
| UPF/N4 状态 | `GET {smf}/nsmf-oam/v1/user-plane-info` 的 `upNodes[*].status.pfcpAssociationSetup` + 对 N4 `IP:8805` 探活（go-upf/C 版 UPF） | `smf/internal/context/user_plane_information.go`、`go-upf/internal/pfcp` |
| UE 监控 | `GET {amf}/namf-oam/v1/registered-ue-context[/{supi}]` | `amf/internal/sbi/producer/oam.go`（supi/cmState/ul&dlCount/算法等） |
| PDU 会话详情 | `GET {smf}/nsmf-oam/v1/ue-pdu-session-info/{ref}` | `smf/internal/context/sm_context.go` OAM 转储 |
| NRF 发现验证 | `GET {nrf}/nnrf-disc/v1/nf-instances?target-nf-type=…` | `nrf/internal/sbi/processor/nf_discovery.go` |
| 控制操作（二次确认） | `POST {amf}/namf-comm/v1/release?supi=&pdusessionid=-1`；`POST {smf}/nsmf-pdusession/v1/sm-contexts/{ref}/release` | `amf/internal/sbi/api_communication.go`、SMF `api_pdusession.go` |
| SBI URI 解析 | Fqdn → apiPrefix → ipEndPoints 优先级 | 复刻 `webconsole/backend/webui_context/context.go` 的 getNfOamUri |
| 配置管理 | `config/<nf>cfg.yaml` 直读 | 各 NF `pkg/factory/config.go` |
| 版本白名单 | Nrf 1.0.2 / Amf 1.0.9 / Smf 1.0.7 / Upf 1.0.3 等 | govalidator `in(x.y.z)` 强校验，不匹配启动失败 |

## 审计规则（演示示例配置自带 1 个真实缺陷）

版本号强校验 → nrfUri 一致性 → 配置存在但未注册（进程未起）→ SMF nodeID vs UPF PFCP 监听 →
DNN 三处一致（AMF/SMF/UPF）→ **UE 地址池（官方样例默认 smfcfg `10.60.0.0/16` 与 upfcfg `60.60.0.0/24`
不同网段，必然标黄）** → N4 关联态 → oauth/明文 http 提示。

## 构建与运行

```bash
# 国内网络建议先 GOPROXY=https://goproxy.cn,direct
go mod tidy && go build -o bin/free5gc-oms .

# 与 free5GC 同机（如 WSL2，NRF=127.0.0.10:8000）：
./bin/free5gc-oms --nrf http://127.0.0.10:8000 --config-dir ~/free5gc/config --user admin --password '***'
# 打开 http://127.0.0.1:3000（端口可 --port 改）
```

`--mock=auto`（默认）：NRF 可达用实数据，不可达自动演示数据；`force` 强制演示；`live` 禁演示。
`--user/--password` 同时提供即启用 Basic-Auth（代码见 main.go `basicAuth`）。

## 接口清单（前后端契约，与网页一一对应）

```
GET  /api/overview /api/nfs /api/nfs/{id} /api/ues /api/ues/{supi}
GET  /api/sessions/{smContextRef}     GET  /api/discovery?target=&requester=
GET  /api/topology                    GET  /api/events?limit=
GET  /api/configs /api/configs/{nf}   GET  /api/audit /api/selftest
POST /api/actions/detach   {supi}
POST /api/actions/pdu-release {supi,pduSessionId,smContextRef}
```

## 已知边界（对应 free5GC 代码现实）

1. **NRF `oauth:true` 时不取 token**：OAM/NFM 返回 401 会被事件页明示（官方 webconsole 同样不支持客户端侧 token，仅 NRF 自签）。
2. UPF（go-upf）无 SBI/不注册 NRF——其状态必须借 SMF 的 N4 视图，这是正确架构行为而非缺陷。
3. free5GC 不支持运行期改 NF 配置（无 reload 接口），配置页为**只读**；下发配置=改 yaml+重启 NF，与官方一致。
4. 前端每 5s 轮询、后端数据缓存 4s（collector.go `cacheTTL`），与 NRF 无过期机制的代价成比例，避免压垮 NF。
5. TLS 自签跳过校验（同 webconsole httpsClient）；公网暴露请用反代 + Basic-Auth。

## 文件

```
main.go          入口：flag/embed 静态/Basic-Auth/HTTP1.1 服务器
client.go        SBI 客户端（GET/POST/探活/serviceURI）
collector.go     采集编排+缓存+事件环+全部路由（live/mock 同构 JSON）
configs.go       <nf>cfg.yaml 读取/摘要/审计（8 组规则）
configsamples.go 内置演示配置（与官方 samples 同构）
mock.go          演示数据（NRF profiles/UE/会话/拓扑，字段名同真实响应）
web/             原生 SPA（从 Python 版移植，契约一致）
```
