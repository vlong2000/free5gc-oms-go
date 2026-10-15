package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const cacheTTL = 4 * time.Second

// Collector 持有连接参数、缓存与事件环；单实例并发安全。
type Collector struct {
	opt     Options
	http    *http.Client
	mu      sync.Mutex
	cache   map[string]cacheEntry
	events  []map[string]any
	lastSt  map[string]string // nfType → up/down（变化才落事件）
	nrfList []map[string]any  // 原始 profile
	nrfMode string            // live | mock | offline
}

type cacheEntry struct {
	t time.Time
	v any
}

func NewCollector(opt Options) *Collector {
	c := &Collector{
		opt:    opt,
		http:   &http.Client{Timeout: 3 * time.Second, Transport: defaultTLS()},
		cache:  map[string]cacheEntry{},
		lastSt: map[string]string{},
	}
	c.emit("info", "OMS", fmt.Sprintf("启动：NRF=%s config=%s mock=%s", opt.NRFUri,
		coalesce(opt.ConfigDir, "<内置示例>"), opt.Mock))
	return c
}

// ---------------- 基础工具 ----------------

func (c *Collector) emit(level, src, msg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append([]map[string]any{{
		"ts": time.Now().Format("15:04:05"), "level": level, "source": src, "msg": msg,
	}}, c.events...)
	if len(c.events) > 200 {
		c.events = c.events[:200]
	}
}

func (c *Collector) cached(key string, fn func() any) any {
	c.mu.Lock()
	if e, ok := c.cache[key]; ok && time.Since(e.t) < cacheTTL {
		c.mu.Unlock()
		return e.v
	}
	c.mu.Unlock()
	v := fn()
	c.mu.Lock()
	c.cache[key] = cacheEntry{time.Now(), v}
	c.mu.Unlock()
	return v
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": msg})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

// ---------------- NRF ----------------

// nrfOnce 拉取 /nnrf-nfm/v1/nf-instances，存 profile 与 mode 标记。
func (c *Collector) nrfOnce() {
	c.mu.Lock()
	if c.nrfList != nil && time.Since(c.cache["nrf-init"].t) < cacheTTL {
		c.mu.Unlock()
		return
	}
	if hit, ok := c.cache["nrf-init"]; ok && time.Since(hit.t) < cacheTTL {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	url := strings.TrimRight(c.opt.NRFUri, "/") + "/nnrf-nfm/v1/nf-instances"
	st, body, err := sbiGet(url, 2500*time.Millisecond)
	list := []map[string]any{}
	mode := "mock"
	if err == nil && st == 200 {
		var raw []map[string]any
		if json.Unmarshal(body, &raw) == nil && len(raw) >= 0 && raw != nil {
			list = raw
			mode = "live"
		}
	} else if c.opt.Mock == "live" {
		mode = "offline"
		c.emit("warn", "NRF", fmt.Sprintf("NRF 不可用（%v）且 --mock=off", err))
	}
	if mode == "mock" {
		list = mockInstances()
		c.emit("info", "NRF", "NRF 未就绪：使用内置演示注册表（--mock=auto）")
	} else {
		c.emit("info", "NRF", fmt.Sprintf("拉取注册表成功，%d 个实例", len(list)))
	}
	c.mu.Lock()
	c.nrfList, c.nrfMode = list, mode
	c.cache["nrf-init"] = cacheEntry{time.Now(), true}
	c.mu.Unlock()
}

func (c *Collector) instances() ([]map[string]any, string) {
	c.nrfOnce()
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.nrfList, c.nrfMode
}

func (c *Collector) mode() string { _, m := c.instances(); return m }

// ---------------- 网元 ----------------

// Nfs：每个注册 NF + UPF 视图（UPF 不注册 NRF，从 SMF user-plane-info 补）。
func (c *Collector) Nfs() []map[string]any {
	profs, mode := c.instances()
	mock := mode != "live"
	out := []map[string]any{}
	for _, p := range profs {
		nfType := str(p["nfType"])
		base := ""
		if svcs, ok := p["nfServices"].([]any); ok && len(svcs) > 0 {
			if s0, ok := svcs[0].(map[string]any); ok {
				base = serviceURI(p, str(s0["serviceName"]))
			}
		}
		health := sbiSelfTest(base + "/")
		if mock {
			health = map[string]any{"ok": true, "status": 404, "ms": 0.3}
		}
		st := "down"
		if b, _ := health["ok"].(bool); b {
			st = "up"
		}
		if prev, had := c.lastSt[nfType]; had && prev != st {
			lvl := "info"
			if st == "down" {
				lvl = "error"
			}
			c.emit(lvl, "健康", fmt.Sprintf("%s SBI %s→%s（%s）", strings.ToUpper(nfType), prev, st, base))
		}
		c.lastSt[nfType] = st
		services := []map[string]any{}
		if svcs, ok := p["nfServices"].([]any); ok {
			for _, s := range svcs {
				svc, _ := s.(map[string]any)
				name := str(svc["serviceName"])
				if name == "" {
					if ns, ok := svc["servicenames"].([]any); ok && len(ns) > 0 {
						name, _ = ns[0].(string)
					}
				}
				services = append(services, map[string]any{
					"name": name, "status": svc["nfServiceStatus"], "uri": serviceURI(p, name),
				})
			}
		}
		out = append(out, map[string]any{
			"nfType": nfType, "id": str(p["nfInstanceId"]), "status": str(p["nfStatus"]),
			"health": health, "ipv4": p["ipv4Addresses"], "fqdn": p["fqdn"],
			"priority": p["priority"], "capacity": p["capacity"], "load": p["load"],
			"snssais": p["snssais"], "services": services,
		})
	}
	for _, u := range c.upfRows() {
		out = append(out, map[string]any{
			"nfType": "UPF", "id": u["name"], "status": "N4",
			"health": map[string]any{"ok": u["assoc"], "ms": u["tcpRtt"],
				"status": tern(u["assoc"] == true, "PFCP-ASSOC", "PFCP-DOWN")},
			"ipv4":     []any{strings.Split(str(u["pfcpAddr"]), ":")[0]},
			"services": []any{}, "snssais": []any{}, "_upf": []any{u},
			"note": "UPF 不注册 NRF；状态来自 SMF user-plane-info + TCP 8805 探活",
		})
	}
	return out
}

func (c *Collector) upfRows() []map[string]any {
	upi := c.Userplane()
	out := []map[string]any{}
	if upi == nil {
		return out
	}
	nodes, _ := upi["upNodes"].(map[string]any)
	for name := range nodes {
		n, _ := nodes[name].(map[string]any)
		if str(n["type"]) != "UPF" {
			continue
		}
		nid := ""
		if o, ok := n["nodeID"].(map[string]any); ok {
			nid = str(o["id"])
		}
		addr := str(n["pfcpAddr"])
		if addr == "" {
			addr = nid + ":8805"
		}
		assoc := true
		if st, ok := n["status"].(map[string]any); ok && st["pfcpAssociationSetup"] == false {
			assoc = false
		}
		rtt := 0.0
		if c.mode() == "live" {
			var ok bool
			ok, rtt = tcpProbe(addr, 1500*time.Millisecond)
			assoc = assoc && ok
		}
		out = append(out, map[string]any{
			"name": name, "pfcpAddr": addr, "assoc": assoc, "tcpRtt": round1(rtt),
		})
	}
	return out
}

// ---------------- UE / 会话 ----------------

func (c *Collector) Ues() []map[string]any {
	list, mode := c.instances()
	if mode != "live" {
		return mockUEs()
	}
	out := []map[string]any{}
	for _, p := range list {
		if str(p["nfType"]) != "AMF" {
			continue
		}
		base := serviceURI(p, "namf-oam")
		if base == "" {
			c.emit("warn", "UE", "该 AMF 未注册 namf-oam 服务（检查 serviceNameList）")
			continue
		}
		st, body, err := sbiGet(base+"/registered-ue-context", 3*time.Second)
		if err != nil {
			c.emit("warn", "UE", fmt.Sprintf("AMF OAM 请求失败 %v", err))
			continue
		}
		if st == 401 {
			c.emit("error", "UE", "NRF 开启 OAuth：OAM 需 bearer token，本工具未内置 client 凭证")
			continue
		}
		if st != 200 {
			c.emit("warn", "UE", fmt.Sprintf("AMF OAM 返回 %d", st))
			continue
		}
		var ues []map[string]any
		_ = json.Unmarshal(body, &ues)
		for _, u := range ues {
			u["amf"] = guamiOf(p)
			out = append(out, u)
		}
	}
	return out
}

func guamiOf(p map[string]any) string {
	ai, _ := p["amfInfo"].(map[string]any)
	g, _ := ai["guamiList"].([]any)
	if len(g) == 0 {
		return ""
	}
	gr, _ := g[0].(map[string]any)
	pl, _ := gr["plmnId"].(map[string]any)
	return fmt.Sprintf("%s-%s/%s", str(pl["mcc"]), str(pl["mnc"]), str(gr["amfId"]))
}

func (c *Collector) UeDetail(supi string) map[string]any {
	for _, u := range c.Ues() {
		if str(u["supi"]) == supi {
			return u
		}
	}
	return nil
}

func (c *Collector) Session(ref string) map[string]any {
	if c.mode() != "live" {
		return mockSession(ref)
	}
	list, _ := c.instances()
	for _, p := range list {
		if str(p["nfType"]) != "SMF" {
			continue
		}
		base := serviceURI(p, "nsmf-oam")
		st, body, err := sbiGet(base+"/ue-pdu-session-info/"+ref, 3*time.Second)
		if err == nil && st == 200 {
			var m map[string]any
			_ = json.Unmarshal(body, &m)
			return m
		}
	}
	return nil
}

func (c *Collector) Userplane() map[string]any {
	return c.cached("upi", func() any {
		if c.mode() != "live" {
			return mockUserplane()
		}
		list, _ := c.instances()
		for _, p := range list {
			if str(p["nfType"]) != "SMF" {
				continue
			}
			base := serviceURI(p, "nsmf-oam")
			st, body, err := sbiGet(base+"/user-plane-info", 3*time.Second)
			if err == nil && st == 200 {
				var m map[string]any
				_ = json.Unmarshal(body, &m)
				return m
			}
		}
		return nil
	}).(map[string]any)
}

func (c *Collector) Discovery(target, requester string) map[string]any {
	url := c.opt.NRFUri + "/nnrf-disc/v1/nf-instances?target-nf-type=" + target +
		"&requester-nf-type=" + requester
	list, mode := c.instances()
	items := []map[string]any{}
	if mode == "live" {
		st, body, err := sbiGet(url, 4*time.Second)
		if err != nil {
			return map[string]any{"error": err.Error(), "url": url}
		}
		var res map[string]any
		_ = json.Unmarshal(body, &res)
		if listv, ok := res["nfInstances"].([]any); ok && st == 200 {
			for _, p := range listv {
				prof, _ := p.(map[string]any)
				items = append(items, discItem(prof))
			}
		}
		return map[string]any{"mode": mode, "url": url, "items": items}
	}
	for _, p := range list {
		if strings.EqualFold(str(p["nfType"]), target) {
			items = append(items, discItem(p))
		}
	}
	return map[string]any{"mode": "mock", "url": url, "items": items}
}

func discItem(p map[string]any) map[string]any {
	svcs, _ := p["nfServices"].([]any)
	uri := ""
	s := ""
	if len(svcs) > 0 {
		s0, _ := svcs[0].(map[string]any)
		s = str(s0["serviceName"])
	}
	uri = serviceURI(p, s)
	return map[string]any{"nfInstanceId": p["nfInstanceId"], "nfType": p["nfType"],
		"snssais": p["snssais"], "uri": uri}
}

// ---------------- 控制操作（危险，前端二次确认） ----------------

func (c *Collector) Detach(supi string) map[string]any {
	if c.mode() != "live" {
		c.emit("warn", "控制", "[模拟] POST /namf-comm/v1/release?supi="+supi)
		return map[string]any{"ok": true, "mock": true}
	}
	list, _ := c.instances()
	for _, p := range list {
		if str(p["nfType"]) != "AMF" {
			continue
		}
		base := serviceURI(p, "namf-comm")
		if base == "" {
			continue
		}
		st, _, err := sbiPost(base+"/release?supi="+supi+"&pdusessionid=-1", map[string]any{}, 6*time.Second)
		if err == nil && (st == 200 || st == 204) {
			c.emit("warn", "控制", fmt.Sprintf("UE 释放 %s → HTTP %d", supi, st))
			return map[string]any{"ok": true, "http": st}
		}
	}
	return map[string]any{"ok": false, "error": "所有可达 AMF 的 namf-comm/release 均未成功（405=未支持路由 / 401=需 OAuth）"}
}

func (c *Collector) ReleasePdu(supi, ref string, pdu int) map[string]any {
	if c.mode() != "live" {
		c.emit("warn", "控制", "[模拟] POST SMF ReleaseSMContext "+ref)
		return map[string]any{"ok": true, "mock": true}
	}
	list, _ := c.instances()
	for _, p := range list {
		if str(p["nfType"]) != "SMF" {
			continue
		}
		base := serviceURI(p, "nsmf-pdusession")
		body := map[string]any{"supi": supi, "pduSessionId": pdu,
			"releaseCause": map[string]any{"cause": "REL_DUE_TO_OAM_INTERVENTION"}}
		st, _, err := sbiPost(base+"/sm-contexts/"+ref+"/release", body, 6*time.Second)
		if err == nil && (st == 200 || st == 204) {
			c.emit("warn", "控制", fmt.Sprintf("PDU 释放 %s/%d → HTTP %d", supi, pdu, st))
			return map[string]any{"ok": true}
		}
	}
	return map[string]any{"ok": false, "error": "SMF release 失败（确认 nsmf-pdusession 与 ref 正确）"}
}

// ---------------- 总览 ----------------

func (c *Collector) Overview() map[string]any {
	profs, mode := c.instances()
	byType := map[string]int{}
	for _, p := range profs {
		byType[strings.ToLower(str(p["nfType"]))]++
	}
	ues := c.Ues()
	connected := 0
	sessions := 0
	for _, u := range ues {
		if str(u["cmState"]) == "CM_CONNECTED" {
			connected++
		}
		if ps, ok := u["pduSessions"].([]any); ok {
			sessions += len(ps)
		}
	}
	nfs := c.Nfs()
	nfHealth := []map[string]any{}
	upCnt := 0
	for _, n := range nfs {
		h, _ := n["health"].(map[string]any)
		if b, _ := h["ok"].(bool); b {
			upCnt++
		}
		nfHealth = append(nfHealth, map[string]any{"nf": n["nfType"], "health": h["ok"],
			"http": h, "addr": n["ipv4"]})
	}
	upi := c.Userplane()
	upfs := []map[string]any{}
	upPaths := []any{}
	if upi != nil {
		nodes, _ := upi["upNodes"].(map[string]any)
		for k := range nodes {
			n, _ := nodes[k].(map[string]any)
			if str(n["type"]) != "UPF" {
				continue
			}
			st, _ := n["status"].(map[string]any)
			pool := []any{}
			for _, x := range orEmpty(n["snssaiDnnPools"]) {
				m, _ := x.(map[string]any)
				if pp := m["pool"]; pp != nil {
					pool = append(pool, pp.([]any)...)
				}
			}
			nid := ""
			if o, ok := n["nodeID"].(map[string]any); ok {
				nid = str(o["id"])
			}
			upfs = append(upfs, map[string]any{"name": k,
				"assoc": !(st != nil && st["pfcpAssociationSetup"] == false),
				"addr":  nid, "pools": pool})
		}
		upPaths = orEmpty(upi["upPaths"])
	}
	return map[string]any{
		"mode": mode,
		"nrf":  map[string]any{"uri": c.opt.NRFUri, "reachable": mode == "live"},
		"counts": map[string]any{"registered": len(profs), "byType": byType,
			"ues": len(ues), "connected": connected, "sessions": sessions},
		"nfHealth": nfHealth, "upfs": upfs, "upPaths": upPaths,
		"refresh": time.Now().Format("15:04:05"), "up": upCnt,
	}
}

// ---------------- 路由 ----------------

func (c *Collector) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		limit := 80
		if l := r.URL.Query().Get("limit"); l != "" {
			if n := atoiSafe(l); n > 0 && n <= 200 {
				limit = n
			}
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		out := c.events
		if len(out) > limit {
			out = out[:limit]
		}
		writeJSON(w, map[string]any{"list": out})
	})
	mux.HandleFunc("/api/overview", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, c.Overview())
	})
	mux.HandleFunc("/api/nfs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"list": c.Nfs()})
	})
	mux.HandleFunc("/api/nfs/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		for _, n := range c.Nfs() {
			if str(n["id"]) == id {
				writeJSON(w, n)
				return
			}
		}
		writeErr(w, 404, "nf instance not found")
	})
	mux.HandleFunc("/api/ues", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"list": c.Ues()})
	})
	mux.HandleFunc("/api/ues/{supi}", func(w http.ResponseWriter, r *http.Request) {
		d := c.UeDetail(r.PathValue("supi"))
		if d == nil {
			writeErr(w, 404, "UE not found")
			return
		}
		writeJSON(w, d)
	})
	mux.HandleFunc("/api/sessions/{ref...}", func(w http.ResponseWriter, r *http.Request) {
		d := c.Session(strings.TrimPrefix(r.PathValue("ref"), "/"))
		if d == nil {
			writeErr(w, 404, "sm context ref not found")
			return
		}
		writeJSON(w, d)
	})
	mux.HandleFunc("/api/topology", func(w http.ResponseWriter, r *http.Request) {
		m := c.Userplane()
		if m == nil {
			m = map[string]any{"upNodes": map[string]any{}, "upPaths": []any{}}
		}
		writeJSON(w, m)
	})
	mux.HandleFunc("/api/discovery", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		writeJSON(w, c.Discovery(q.Get("target"), orElse(q.Get("requester"), "NRF")))
	})
	mux.HandleFunc("/api/configs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, c.Configs().Overview())
	})
	mux.HandleFunc("/api/configs/{nf}", func(w http.ResponseWriter, r *http.Request) {
		if d := c.Configs().By(r.PathValue("nf")); d != nil {
			writeJSON(w, d)
			return
		}
		writeErr(w, 404, "no such config")
	})
	mux.HandleFunc("/api/audit", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, c.Audit())
	})
	mux.HandleFunc("/api/actions/detach", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Supi string }
		_ = json.NewDecoder(r.Body).Decode(&b)
		if b.Supi == "" {
			writeErr(w, 400, "supi required")
			return
		}
		writeJSON(w, c.Detach(b.Supi))
	})
	mux.HandleFunc("/api/actions/pdu-release", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Supi         string
			SMContextRef string
			PduSessionID int
		}
		_ = json.NewDecoder(r.Body).Decode(&b)
		writeJSON(w, c.ReleasePdu(b.Supi, b.SMContextRef, b.PduSessionID))
	})
	mux.HandleFunc("/api/selftest", func(w http.ResponseWriter, r *http.Request) {
		_, st := c.instances()
		writeJSON(w, map[string]any{"checks": []any{
			map[string]any{"item": "NRF 可达/数据源", "result": st},
			map[string]any{"item": "配置目录", "result": orElse(c.opt.ConfigDir, "<内置示例>")},
			map[string]any{"item": "演示模式", "result": c.opt.Mock},
			map[string]any{"item": "前端资源", "result": true},
		}})
	})
	mux.HandleFunc("/api/probe", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ URL string }
		_ = json.NewDecoder(r.Body).Decode(&b)
		log.Println("probe", b.URL)
		writeJSON(w, sbiSelfTest(b.URL))
	})
}

// ---------------- 泛型小工具 ----------------

func str(v any) string {
	s, _ := v.(string)
	return s
}
func orEmpty(v any) []any {
	a, _ := v.([]any)
	return a
}
func orElse(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
func tern(b bool, a, c2 string) string {
	if b {
		return a
	}
	return c2
}
func atoiSafe(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}
