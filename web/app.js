/* free5GC 网管中心 —— 原生 JS，无构建 */
"use strict";
const $ = (s, el = document) => el.querySelector(s);
const $$ = (s, el = document) => [...el.querySelectorAll(s)];
const esc = x => x == null ? "" : String(x).replace(/[&<>"]/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
const get = async url => (await fetch(url)).json();
const post = async (url, body) => (await fetch(url, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) })).json();

const render = { current: "ov" };

/* ---------------- tabs ---------------- */
$$("#tabs button").forEach(b => b.onclick = () => {
  $$("#tabs button").forEach(x => x.classList.toggle("on", x === b));
  $$(".view").forEach(v => v.classList.toggle("on", v.id === "v-" + b.dataset.v));
  render.current = b.dataset.v;
  tick(true);
});

/* ---------------- helpers ---------------- */
const dot = ok => `<span class="dot ${ok === true ? "ok" : ok === false ? "down" : "na"}"></span>`;
const chip = t => `<span class="chip">${esc(t)}</span>`;
const svcChip = t => `<span class="chip svc">${esc(t)}</span>`;
function fillTable(el, head, rows) {
  const t = $(el);
  if (!rows.length) { t.innerHTML = `<tr><td class="empty" colspan="${head.length}">（无数据）</td></tr>`; return; }
  t.innerHTML = `<thead><tr>${head.map(h => `<th>${h}</th>`).join("")}</tr></thead><tbody>` +
    rows.map(r => `<tr>${r.map(c => `<td>${c}</td>`).join("")}</tr>`).join("") + `</tbody>`;
}
function toast(msg) {
  const t = document.createElement("div");
  t.textContent = msg;
  Object.assign(t.style, { position: "fixed", right: "18px", bottom: "18px", background: "#101a2e", border: "1px solid #43d3e8", padding: "9px 14px", zIndex: 99, borderRadius: 8 });
  document.body.appendChild(t); setTimeout(() => t.remove(), 4200);
}

/* ---------------- 总览 ---------------- */
async function renderOverView() {
  const ov = await get("/api/overview");
  $("#banner-mock").classList.toggle("hidden", ov.mode !== "mock");
  $("#nrfline").textContent = `NRF ${ov.nrf.uri} ${ov.nrf.reachable ? "· 可达" : "· 不可达"}`;
  $("#refreshline").textContent = "刷新 " + ov.refresh;
  const c = ov.counts;
  $("#kpis").innerHTML = [
    [`注册网元类型`, Object.keys(c.byType || {}).map(n => chip(n)).join("")],
    ["SBI 可探活", (ov.nfHealth || []).filter(n => n.health).length + " / " + (ov.nfHealth || []).length],
    ["UE（在线/总）", `${c.connected} / ${c.ues}`],
    ["PDU 会话", c.sessions],
    ["UPF", (ov.upfs || []).map(u => chip(u.name + (u.assoc ? "·N4✓" : "·N4✗"))).join("") || chip("-")],
  ].map(([k, v]) => `<div class="kpi"><b>${v}</b><span>${esc(k)}</span></div>`).join("");
  fillTable("#healthtbl", ["网元", "状态/延迟", "IPv4", "发现"], (ov.nfHealth || []).map(n => [
    `${dot(n.health)} <b>${esc(n.nf)}</b>`,
    n.health ? `GET / → ${esc(n.http.status ?? "OK")}${n.http.ms ? " · " + n.http.ms + " ms" : ""}` : (n.http.error ? "DOWN: " + esc(n.http.error) : "DOWN"),
    `<span class="mono">${esc((n.addr || []).join() || "-")}</span>`,
    `<button class="btn sm" data-goto="disc" data-t="${esc(n.nf)}">模拟被消费</button>`,
  ]));
  $$("[data-goto]").forEach(x => x.onclick = () => {
    $$("#tabs button").find(b => b.dataset.v === "disc").click();
    if ($("#disc-target")) { $("#disc-target").value = x.dataset.t; doDisc(); }
  });
  const upfs = ov.upfs || [];
  $("#uptopo").innerHTML = upfs.length
    ? upfs.map(u => `<div class="upnode"><span class="dot ${u.assoc ? "ok" : "down"}"></span>
        <b>${esc(u.name)}</b> · nodeID <code>${esc(u.addr || "?")}:8805</code>（PFCP/N4）
        · 关联 ${u.assoc ? "ESTABLISHED" : "DOWN"} · pools ${(u.pools || []).map(x => `<code>${esc(x)}</code>`).join(" ") || "-"}</div>`).join("")
      + (ov.upPaths || []).map(p => `<div class="pathline">DataPath: ${p.map(esc).join(" ⇄ ")}</div>`).join("")
    : `<div class="empty">未取到 SMF /nsmf-oam/v1/user-plane-info（确认 SMF 在跑且 nsmf-oam 已启用）</div>`;
  const cfg = await get("/api/configs");
  const amf = (cfg.items || []).find(i => i.nf === "amf" && i.summary) || {};
  $("#slicebox").innerHTML = amf.summary ? `
    <div class="chips" style="margin-bottom:8px">S-NSSAI ${(amf.summary.snssai || []).map(chip).join("")}
      &nbsp;PLMN ${(amf.summary.plmn || []).map(chip).join("")}
      &nbsp;DNN ${(amf.summary.dnn || []).map(chip).join("")}</div>
    GUAMI <code>${esc((amf.summary.guami || ["-"]).join(" ; "))}</code><br>
    TAI ${(amf.summary.tai || []).map(chip).join("") || esc("-")} ·
    完整性 ${(amf.summary.integrity || []).join("/")} / 加密 ${(amf.summary.ciphering || []).join("/")}`
    : `<div class="empty">读取 AMF 配置失败</div>`;
  const ev = await get("/api/events?limit=6");
  $("#ovev").innerHTML = (ev.list || []).length ? ev.list.map(e =>
    `<div class="mono" style="font-size:12px;padding:3px 0;border-bottom:1px solid #131e34">
      ${esc(e.ts)} <b style="color:${e.level === "error" ? "#ff5d6c" : e.level === "warn" ? "#f5b83d" : "#43d3e8"}">[${e.level}]</b>
      ${esc(e.source)}: ${esc(e.msg)}</div>`).join("") : `<div class="empty">无事件</div>`;
}

/* ---------------- 网元 ---------------- */
async function renderNf() {
  const { list } = await get("/api/nfs");
  $("#nfgrid").innerHTML = (list || []).map(n => `
    <details class="nfc">
      <summary>${dot(n.health.ok)} <b>${esc(n.nfType)}</b>
        <span class="muted mono" style="font-size:12px">${esc((n.ipv4 || [])[0] || (n._upf || [])[0]?.pfcpAddr || "")}</span>
        <span class="pill" style="margin-left:auto;background:#122036;color:#8fe7f8">${esc(n.status || "-")}</span></summary>
      <div class="nfb">
        ${n.note ? `<div class="row"><span>来源</span><span>${esc(n.note)}</span></div>` : ""}
        ${n._upf ? `<div class="row"><span>PFCP/探测</span><span>${(n._upf || []).map(u => `${esc(u.name)} ${esc(u.pfcpAddr)} assoc=${u.assoc ? "✓" : "✗"} tcp=${u.tcpRtt ?? "-" }ms`).join("<br>")}</span></div>` : ""}
        <div class="row"><span>实例 ID</span><span class="mono" style="font-size:11px">${esc((n.id || "").slice(0, 24))}${(n.id || "").length > 24 ? "…" : ""}</span></div>
        ${n.health.status || n.health.ms ? `<div class="row"><span>SBI 探活</span><span>${esc(n.health.status ?? "tcp")} · ${esc(n.health.ms)}ms</span></div>` : ""}
        ${(n.snssais || []).length ? `<div class="row"><span>s-NSSAI</span><span class="chips">${n.snssais.map(s => `<code>${esc(s.sst)}·${esc(s.sd || "-")}</code>`).join(" ")}</span></div>` : ""}
        <div class="row"><span>priority/capacity/load</span><span>${esc(n.priority)} / ${esc(n.capacity)} / ${esc(n.load)}</span></div>
        <div class="row"><span>允许类型</span><span mut class="muted">—</span></div>
        <div style="margin-top:10px">
          ${(n.services || []).map(s => `<div class="svcrow">${dot(s.status === "REGISTERED")}
            <code>${esc(s.name)}</code>
            <a class="muted mono" style="font-size:11px" href="${esc(s.uri)}" target="_blank" rel="noopener">${esc((s.uri || "").replace(/^https?:\/\//, ""))}</a></div>`).join("")
            || '<div class="muted">无 SBI 服务声明</div>'}
        </div>
      </div>
    </details>`).join("") || `<div class="empty">NRF 注册表为空</div>`;
}

/* ---------------- UE 与会话 ---------------- */
async function sessionsHtml(u) {
  const rows = [];
  for (const ps of (u.pduSessions || [])) {
    const s = await get("/api/sessions/" + encodeURIComponent(ps.smContextRef));
    rows.push({ ps, s: s && !s.error ? s : null });
  }
  if (!rows.length) return "";
  return (rows.map(({ ps, s }) => `<tr>
      <td><b>#${esc(ps.pduSessionId)}</b></td><td>${esc(s?.upCnxState || "-")}</td>
      <td>${esc(s?.dnn || "-")} / ${esc(s?.sNssai?.sst ?? "-")}${s?.sNssai?.sd ? "/" + esc(s.sNssai.sd) : ""}</td>
      <td class="mono">${esc(s?.ueIP || s?.pduAddress || "-")}</td>
      <td>${esc(s?.selectedUPF || "-")} · N3 TEID ${esc(s?.upN3TeID ?? "-")}<br><small class="muted">seidL ${esc((s?.localSEID || "-").slice(-6))} · seidR ${esc((s?.remoteSEID || "-").slice(-6))}</small></td>
      <td>${(s?.qosFlows || []).map(q => `5QI ${q["5qi"]}/QFI ${q.qfi}`).join(" ") || "-"}<br>
          <small class="muted">AMBR ${esc((s?.sessionRule?.authSessAMBR || {}).uplink || "-")}／${esc((s?.sessionRule?.authSessAMBR || {}).downlink || "-")}</small></td>
      <td class="mono" style="font-size:11px">${esc((ps.smContextRef || "").slice(-12))}
          ${s ? `<details><summary class="muted" style="font-size:11px">原始</summary><pre class="json">${esc(JSON.stringify(s, null, 1))}</pre></details>` : ""}</td>
      <td><button class="btn sm danger" data-rel="${esc(ps.smContextRef)}" data-pdu="${esc(ps.pduSessionId)}" data-supi="${esc(u.supi)}">释放</button></td>
    </tr>`).join(""));
}
async function renderUes() {
  const { list } = await get("/api/ues");
  fillTable("#uetbl", ["SUPI", "CM", "接入/RAT", "会话", "完整性/加密", "操作"],
    (list || []).map((u, i) => [
      `<b>${esc(u.supi)}</b><br><small class="muted">${esc(u.authMethod || "")} · PLMN ${esc((u.servingNetworkName || "").slice(4, 10))}</small>`,
      `<span class="pill ${esc(u.cmState)}">${esc((u.cmState || "-").replace("CM_", ""))}</span>`,
      esc((u.accessType || "-").replace("_ACCESS", "")) + (u.ratType ? "/" + esc(u.ratType) : ""),
      (u.pduSessions || []).map(s => `#${s.pduSessionId}`).join(" ") || "-",
      `${esc(u.integrityEnabledAlgorithm || "-")} / ${esc(u.cipheringEnabledAlgorithm || "-")}`,
      `<button class="btn sm" data-ue="${i}">详情</button>
       <button class="btn sm danger" data-act="detach" data-supi="${esc(u.supi)}">释放 UE</button>`,
    ]));
  $$("#uetbl [data-ue]").forEach(b => b.onclick = async () => {
    const u = list[+b.dataset.ue];
    const d = $("#uedetail");
    d.classList.remove("hidden");
    d.innerHTML = `<h4 class="mono" style="margin:2px 0 8px">${esc(u.supi)} · ${esc(u.cmState)}
      <small class="muted">PEI ${esc(u.pei || "-")} · UL/DL 计数 ${esc(u.ulCount)} / ${esc(u.dlCount)} · ${esc(u.ratType || "")}</small></h4>
      <table class="tbl"><thead><tr><th>PDU</th><th>UP 状态</th><th>DNN/NSSAI</th><th>UE IP</th><th>UPF/隧道</th><th>QoS</th><th>SMF ref</th><th></th></tr></thead>
      <tbody>${await sessionsHtml(u)}</tbody></table>`;
    $$("#uedetail [data-rel]").forEach(x => x.onclick = () => doRelease(x));
  });
  $$("#uetbl [data-act=detach]").forEach(b => b.onclick = () => doDetach(b));
}
async function doDetach(b) {
  if (!confirm(`确认对 ${b.dataset.supi} 执行 UE 上下文释放？\n（POST /namf-comm/v1/release?supi=...&pdusessionid=-1，会真实打断该 UE）`)) return;
  const r = await post("/api/actions/detach", { supi: b.dataset.supi });
  toast(r.ok ? (r.mock ? "[模拟] UE 释放已下发" : "UE 释放已下发") : "UE 释放失败：" + (r.error || "?"));
}
async function doRelease(b) {
  if (!confirm(`确认释放 PDU 会话 #${b.dataset.pdu}？\n（POST /nsmf-pdusession/v1/sm-contexts/{ref}/release，原因 oam-intervention）`)) return;
  const r = await post("/api/actions/pdu-release", { supi: b.dataset.supi, pduSessionId: +b.dataset.pdu, smContextRef: b.dataset.rel });
  toast(r.ok ? (r.mock ? "[模拟] PDU 会话释放已下发" : "PDU 会话释放已下发") : "释放失败：" + (r.error || "?"));
}

/* ---------------- 配置 ---------------- */
let cfgCache = null;
async function renderCfg(force) {
  if (force || !cfgCache) cfgCache = await get("/api/configs");
  $("#cfgdir").textContent = cfgCache.dir;
  const sel = $("#cfgsel");
  const names = Object.keys(sel.dataset).length ? sel.value : null;
  sel.innerHTML = cfgCache.items.map(i => `<option value="${i.nf}">${esc(i.label)}${i.error ? " ⚠" : ""}</option>`).join("");
  sel.dataset.k = 1;
  sel.value = names && [...sel.options].some(o => o.value === names) ? names : (sel.value || "amf");
  showCfg(sel.value);
}
function showCfg(nf) {
  const it = cfgCache.items.find(x => x.nf === nf) || {};
  const s = it.summary || {};
  const skip = new Set(["version", "sbi", "serviceNameList", "upNodes", "dnnList", "nrfUri", "description"]);
  let html = `<dl class="kv">
    <dt>文件</dt><dd>${esc(it.path)}</dd>
    <dt>Info.Version</dt><dd>${esc(s.version)}</dd>`;
  if (s.sbi) html += `<dt>SBI 服务</dt><dd><code>${esc(s.sbi.uri)}</code>（bind ${esc(s.sbi.bindingIPv4 || "-")} / reg ${esc(s.sbi.registerIPv4 || "-")} · ${esc(s.sbi.scheme || "http")}${/http:/.test(s.sbi.uri) ? ' <span class="muted">(明文)</span>' : ""}</dd>`;
  if (s.PFCP) html += `<dt>PFCP (N4)</dt><dd><code>${esc(s.PFCP.addr || s.PFCP.listenAddr || "?")}</code> · nodeID ${esc(s.PFCP.nodeID || s.PFCP.nodeId || "-")}</dd>`;
  if (s.nrfUri) html += `<dt>nrfUri</dt><dd><code>${esc(s.nrfUri)}</code>${/^http:/.test(s.nrfUri) ? ' <span class="muted">(明文 http)</span>' : ""}</dd>`;
  if (s.serviceNameList?.length) html += `<dt>serviceNameList</dt><dd>${s.serviceNameList.map(svcChip).join(" ")}</dd>`;
  if (s.upNodes?.length) html += `<dt>用户面拓扑(smfcfg)</dt><dd>${s.upNodes.map(n => `<code>${esc(n.name)}[${esc(n.type)}]</code> nodeID=${esc(n.nodeID || "-")} ${esc(n.addr || "")} ${(n.cidrs || []).length ? "cidr " + esc((n.cidrs || []).join(", ")) : ""}`).join("<br>")}</dd>`;
  for (const [k, v] of Object.entries(s)) {
    if (skip.has(k) || v == null || (Array.isArray(v) && !v.length)) continue;
    html += `<dt>${esc(k)}</dt><dd>${typeof v === "object" ? esc(JSON.stringify(v)) : esc(String(v))}</dd>`;
  }
  if (it.error) html += `<div class="alert err"><span class="lvl">ERR</span>解析失败：${esc(it.error)}（需 PyYAML：pip install pyyaml）</div>`;
  $("#cfgsum").innerHTML = html + "</dl>";
  const raw = it.raw || "";
  $("#cfgraw").textContent = raw || "（无原文：真实模式请指向 free5gc/config 目录）";
}
$("#cfgsel")?.addEventListener("change", e => showCfg(e.target.value));
$("#btn-cfg-reload")?.addEventListener("click", () => renderCfg(true));

/* ---------------- 审计 ---------------- */
async function renderAudit() {
  const d = await get("/api/audit");
  const stat = { ok: 0, info: 0, warn: 0, error: 0 };
  (d.items || []).forEach(i => stat[i.level] = (stat[i.level] || 0) + 1);
  $("#auditlist").innerHTML = d.items.map(i =>
    `<div class="alert ${i.level}"><span class="lvl">${i.level.toUpperCase()}</span>
     <div><b>${esc(i.group)}</b>：${i.detail}</div></div>`).join("")
    + `<div class="muted" style="margin-top:10px">共 ${stat.ok} 通过 · ${stat.info} 提示 · ${stat.warn} 告警 · ${stat.error} 错误</div>`;
}

/* ---------------- NRF 发现 ---------------- */
function initDisc() {
  const t = $("#disc-target"), r = $("#disc-req");
  if (t.options.length) return;
  const NFF = ["NRF", "AMF", "SMF", "UPF", "UDM", "UDR", "PCF", "AUSF", "NSSF", "CHF", "BSF", "NEF", "SCP"];
  t.innerHTML = r.innerHTML = NFF.map(n => `<option>${n}</option>`).join("");
  r.value = "AMF"; t.value = "AMF";
  t.onchange = r.onchange = doDisc;
  doDisc();
}
async function doDisc() {
  const d = await get(`/api/discovery?target=${$("#disc-target").value}&requester=${$("#disc-req").value}`);
  $("#disc-url").textContent = (d.mode ? "[" + d.mode + "] " : "") + (d.url || "");
  fillTable("#disctbl", ["nfType", "实例 ID", "解析出的 SBI URI", "s-NSSAI"],
    (d.items || []).map(p => [esc(p.nfType),
      `<span class="mono" style="font-size:11px">${esc((p.nfInstanceId || "").slice(0, 22))}…</span>`,
      `<code>${esc(p.uri || "-")}</code>`,
      (p.snssais || []).map(s => esc(s.sst + (s.sd ? " / " + s.sd : ""))).join(" ") || "-"]));
}
$("#btn-disc")?.addEventListener("click", doDisc);

/* ---------------- 事件视图 ---------------- */
async function renderEv() {
  const d = await get("/api/events?limit=150");
  fillTable("#evtbl", ["时刻", "级别", "来源", "内容"], (d.list || []).map(e => [
    `<span class="mono" style="font-size:12px">${esc(e.ts)}</span>`,
    `<b style="color:${e.level === "error" ? "#ff5d6c" : e.level === "warn" ? "#f5b83d" : "#43d3e8"}">${e.level}</b>`,
    esc(e.source), esc(e.msg)]));
}

/* ---------------- 主循环 ---------------- */
async function tick(force) {
  const r = render.current;
  try {
    if (r === "ov") await renderOverView();
    else if (r === "nf") await renderNf();
    else if (r === "ues-sec") await renderUes();
    else if (r === "cfg") await renderCfg(force);
    else if (r === "audit") await renderAudit();
    else if (r === "disc") initDisc();
    else if (r === "ev") await renderEv();
  } catch (e) {
    console.error(e);
    const b = $("#banner-mock"); if (b) { b.textContent = "刷新失败: " + e; b.classList.remove("hidden"); }
  }
}
$("#btn-refresh")?.addEventListener("click", () => { cfgCache = null; tick(true); });
setInterval(() => { $("#autoreload")?.checked && tick(false); }, 5000);
tick(true);
