package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// 依据各 NF pkg/factory/config.go：Info.Version 为 govalidator in(x.y.z) 强校验；
// yaml.v2 反序列化大小写不敏感，这里用 getCI 模拟。

var nfLabels = map[string]string{
	"nrf": "NRF 服务注册中心", "amf": "AMF 接入与移动性", "smf": "SMF 会话管理",
	"upf": "UPF 用户面", "udm": "UDM 归属服务器", "udr": "UDR 数据仓库",
	"pcf": "PCF 策略控制", "nssf": "NSSF 切片选择", "ausf": "AUSF 鉴权",
	"chf": "CHF 计费", "bsf": "BSF 绑定支持", "nef": "NEF 开放", "scp": "SCP 代理",
	"n3iwf": "N3IWF 非3GPP接入", "tngf": "TNGF 可信接入", "webui": "WebConsole",
	"uerouting": "SMF UE 路由",
}
var expectVersion = map[string]string{
	"amf": "1.0.9", "smf": "1.0.7", "nrf": "1.0.2", "udm": "1.0.6", "udr": "1.0.4",
	"pcf": "1.0.7", "nssf": "1.0.3", "ausf": "1.0.4", "upf": "1.0.3", "chf": "1.0.1",
	"webui": "1.0.3", "bsf": "1.0.2", "nef": "1.0.3", "scp": "1.0.2", "n3iwf": "1.0.3",
}
var coreNFs = []string{"nrf", "amf", "smf", "upf", "udm", "udr", "pcf", "nssf", "ausf"}
var registerNRF = map[string]bool{"amf": true, "smf": true, "udm": true, "udr": true,
	"pcf": true, "nssf": true, "ausf": true, "chf": true, "bsf": true, "nef": true,
	"scp": true, "webui": true, "nrf": true}

type cfgFile struct {
	Path string         `json:"path"`
	Data map[string]any `json:"-"`
	Raw  string         `json:"raw"`
	Err  string         `json:"error"`
}

func getCI(m map[string]any, keys ...string) any {
	if m == nil {
		return nil
	}
	for _, k := range keys {
		lk := strings.ToLower(k)
		for kk, v := range m {
			if strings.ToLower(kk) == lk {
				return v
			}
		}
	}
	return nil
}

func dig(obj any, path ...string) any {
	cur := obj
	for _, p := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = getCI(m, p)
		if cur == nil {
			return nil
		}
	}
	return cur
}

func listOr(v any) []any {
	l, _ := v.([]any)
	return l
}

var cidrRe = regexp.MustCompile(`^\d+\.\d+\.\d+\.\d+/\d+$`)

func walkCidrs(o any, acc map[string]bool) {
	switch v := o.(type) {
	case map[string]any:
		for _, x := range v {
			walkCidrs(x, acc)
		}
	case []any:
		for _, x := range v {
			walkCidrs(x, acc)
		}
	case string:
		if cidrRe.MatchString(v) {
			acc[v] = true
		}
	}
}

func mapOf(v any) map[string]any { m, _ := v.(map[string]any); return m }

// summarize 按 NF 类型提炼网管关注字段（与 Python 版一致）。
func summarize(nf string, data map[string]any) map[string]any {
	cfg := mapOf(getCI(data, "configuration"))
	if cfg == nil {
		cfg = data
	}
	info := mapOf(getCI(data, "info"))
	s := map[string]any{}
	ver := getCI(info, "version")
	if ver == nil {
		ver = getCI(data, "version")
	}
	s["version"] = ver
	switch nf {
	case "upf":
		s["PFCP"] = getCI(cfg, "pfcp")
		s["GTP-U"] = getCI(cfg, "gtpu")
		dl := listOr(getCI(cfg, "dnn_list", "dnnList"))
		s["dnnList"] = dl
		dnns := []any{}
		for _, d := range dl {
			dnns = append(dnns, getCI(mapOf(d), "dnn"))
		}
		s["DNN"] = dnns
	case "n3iwf":
		s["IKE监听"] = getCI(cfg, "ikeBindAddress")
		s["NASTcpPort"] = getCI(cfg, "nasTcpPort")
		s["UPFGtpBind"] = getCI(cfg, "n3iwfGtpBindAddress")
		s["Tunnel"] = getCI(cfg, "ipSecTunnelAddress")
	case "uerouting":
		s["路由组"] = keysOf(getCI(cfg, "ueRoutingInfo"))
	default:
		sbi := mapOf(getCI(cfg, "sbi"))
		scheme, _ := getCI(sbi, "scheme").(string)
		if scheme == "" {
			scheme = "http"
		}
		bindIP, _ := getCI(sbi, "registerIPv4", "registerIPv6", "bindingIPv4").(string)
		if bindIP == "" {
			bindIP = "?"
		}
		port := 8000
		if p, ok := getCI(sbi, "port").(int); ok && p != 0 {
			port = p
		} else if p, ok := getCI(sbi, "port").(float64); ok && p != 0 {
			port = int(p)
		}
		s["name"] = getCI(cfg, "amfName", "smfName", "nrfName", "udmName", "udrName",
			"pcfName", "nssfName", "ausfName", "chfName", "webuiName")
		s["sbi"] = map[string]any{"scheme": scheme, "bindingIPv4": getCI(sbi, "bindingIPv4"),
			"registerIPv4": bindIP, "uri": fmt.Sprintf("%s://%s:%d", scheme, bindIP, port)}
		s["nrfUri"] = getCI(cfg, "nrfUri")
		s["serviceNameList"] = getCI(cfg, "serviceNameList")
		s["oauth"] = getCI(cfg, "oauth")
	}
	switch nf {
	case "amf":
		gu := []any{}
		for _, g := range listOr(getCI(cfg, "servedGuamiList")) {
			p, _ := getCI(mapOf(g), "plmnId", "mcc").(string)
			n, _ := getCI(mapOf(g), "plmnId", "mnc").(string)
			gu = append(gu, fmt.Sprintf("%s-%s/%s", p, n, getCI(mapOf(g), "amfId")))
		}
		s["guami"] = gu
		s["ngapIpList"] = getCI(cfg, "ngapIpList")
		s["ngapPort"] = orVal(getCI(cfg, "ngapPort"), 38412)
		s["plmn"] = plmnList(getCI(cfg, "plmnSupportList"))
		s["dnn"] = getCI(cfg, "supportDnnList")
		s["tai"] = taiList(getCI(cfg, "supportTaiList"))
		snssai := map[string]bool{}
		for _, g := range listOr(getCI(cfg, "plmnSupportList")) {
			for _, x := range listOr(getCI(mapOf(g), "snssaiList")) {
				m := mapOf(x)
				sd, _ := getCI(m, "sd").(string)
				if sd == "" {
					sd = fmt.Sprint(getCI(m, "sd"))
				}
				key := fmt.Sprint(getCI(m, "sst"))
				if sd != "<nil>" && sd != "" {
					key += "/" + sd
				}
				snssai[key] = true
			}
		}
		s["snssai"] = boolKeys(snssai)
		sec := mapOf(getCI(cfg, "security"))
		s["integrity"] = getCI(sec, "integrityOrder")
		s["ciphering"] = getCI(sec, "cipheringOrder")
		tm := map[string]any{}
		for _, k := range []string{"t3502Value", "t3512Value", "non3gppDeregTimerValue"} {
			tm[k] = getCI(cfg, k)
		}
		s["timers"] = tm
	case "smf":
		s["plmn"] = rawPlmnList(getCI(cfg, "plmnList"))
		s["pfcp"] = getCI(cfg, "pfcp")
		spi := []any{}
		for _, x := range listOr(getCI(cfg, "snssaiInfos")) {
			m := mapOf(x)
			// 兼容 sNssai: {..} / sNssaiInfo: {..} / sNssaiInfo: [{..}] 三种写法
			var snss map[string]any
			if v := getCI(m, "sNssai", "sNssaiInfo"); v != nil {
				if l, ok := v.([]any); ok && len(l) > 0 {
					snss = mapOf(l[0])
				} else {
					snss = mapOf(v)
				}
			}
			dnn := []any{}
			for _, d := range listOr(getCI(m, "dnnInfos")) {
				dnn = append(dnn, getCI(mapOf(d), "dnn"))
			}
			spi = append(spi, map[string]any{"sst": getCI(snss, "sst"),
				"sd": getCI(snss, "sd"), "dnn": dnn})
		}
		s["snssaiDnn"] = spi
		upi := mapOf(getCI(cfg, "userplaneInformation"))
		un := []any{}
		for _, u := range listOr(getCI(upi, "upNodes")) {
			m := mapOf(u)
			cid := map[string]bool{}
			walkCidrs(m, cid)
			un = append(un, map[string]any{"name": getCI(m, "name"), "type": getCI(m, "type"),
				"nodeID": getCI(m, "nodeID", "nodeId"), "addr": getCI(m, "addr"),
				"cidrs": boolKeys(cid)})
		}
		s["upNodes"] = un
		s["links"] = listOr(getCI(upi, "links"))
	case "udr":
		s["dbConnector"] = str(orVal(getCI(cfg, "dbConnectorType"), "mongodb"))
		s["mongodb"] = getCI(cfg, "mongodb")
	case "udm":
		s["suciProfile"] = len(listOr(getCI(cfg, "SuciProfile")))
	case "nssf":
		s["nsiList"] = getCI(cfg, "nsiList")
		s["supportedPlmn"] = rawPlmnList(getCI(cfg, "supportedPlmnList"))
	case "webui":
		s["webServer"] = getCI(cfg, "webServer")
		s["mongodb"] = getCI(cfg, "mongodb")
	}
	return s
}

func plmnList(v any) []any {
	out := []any{}
	for _, p := range listOr(v) {
		m, _ := getCI(mapOf(p), "plmnId").(string)
		_ = m
		pl := mapOf(getCI(mapOf(p), "plmnId"))
		out = append(out, fmt.Sprintf("%s-%s", asString(getCI(pl, "mcc")), asString(getCI(pl, "mnc"))))
	}
	return out
}
func rawPlmnList(v any) []any {
	out := []any{}
	for _, p := range listOr(v) {
		m := mapOf(p)
		out = append(out, fmt.Sprintf("%s-%s", asString(getCI(m, "mcc")), asString(getCI(m, "mnc"))))
	}
	return out
}
func taiList(v any) []any {
	out := []any{}
	for _, t := range listOr(v) {
		m := mapOf(t)
		pl := mapOf(getCI(m, "plmnId"))
		out = append(out, fmt.Sprintf("%s-%s/%s", asString(getCI(pl, "mcc")), asString(getCI(pl, "mnc")),
			asString(getCI(m, "tac"))))
	}
	return out
}
func asString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case nil:
		return ""
	case int:
		return fmt.Sprintf("%03d", s)
	case float64:
		return strings.TrimSuffix(fmt.Sprint(s), ".0")
	}
	return fmt.Sprint(v)
}
func boolKeys(set map[string]bool) []string {
	out := []string{}
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func keysOf(v any) []string {
	m := mapOf(v)
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func orVal(v any, d any) any {
	if v != nil {
		return v
	}
	return d
}

// ConfigSets 管理一次读取的配置文件集合。
type ConfigSets struct {
	Dir   string
	Items map[string]cfgFile
}

func loadConfigFiles(dir string) *ConfigSets {
	cs := &ConfigSets{Dir: dir, Items: map[string]cfgFile{}}
	if dir == "" {
		return cs
	}
	dir = expandHome(dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return cs
	}
	for _, e := range entries {
		name := e.Name()
		nf := ""
		switch {
		case strings.HasSuffix(name, "cfg.yaml"):
			nf = strings.TrimSuffix(name, "cfg.yaml")
		case name == "uerouting.yaml":
			nf = "uerouting"
		default:
			continue
		}
		p := filepath.Join(dir, name)
		raw, err := os.ReadFile(p)
		f := cfgFile{Path: p, Raw: string(raw)}
		var v map[string]any
		if err != nil || yaml.Unmarshal(raw, &v) != nil {
			if err == nil {
				err = fmt.Errorf("yaml invalid")
			}
			f.Err = err.Error()
		} else {
			f.Data = v
		}
		cs.Items[nf] = f
	}
	return cs
}

func (c *Collector) Configs() *ConfigSets {
	if c.opt.ConfigDir != "" {
		if cs := loadConfigFiles(c.opt.ConfigDir); len(cs.Items) > 0 {
			return cs
		}
	}
	if c.opt.Mock == "live" {
		return &ConfigSets{Dir: c.opt.ConfigDir, Items: map[string]cfgFile{}}
	}
	return builtinSampleConfigs()
}

func (cs *ConfigSets) Overview() map[string]any {
	names := []string{}
	for nf := range cs.Items {
		names = append(names, nf)
	}
	sort.Strings(names)
	items := []map[string]any{}
	for _, nf := range names {
		f := cs.Items[nf]
		var sum map[string]any
		if f.Data != nil {
			sum = summarize(nf, f.Data)
		}
		raw := f.Raw
		if len(raw) > 8000 {
			raw = raw[:8000]
		}
		label := nfLabels[nf]
		if label == "" {
			label = strings.ToUpper(nf)
		}
		items = append(items, map[string]any{"nf": nf, "label": label, "path": f.Path,
			"error": f.Err, "summary": sum, "raw": raw})
	}
	dir := cs.Dir
	if dir == "" {
		dir = "(内置示例)"
	}
	return map[string]any{"dir": dir, "items": items}
}

func (cs *ConfigSets) By(nf string) map[string]any {
	f, ok := cs.Items[nf]
	if !ok {
		return nil
	}
	var sum map[string]any
	if f.Data != nil {
		sum = summarize(nf, f.Data)
	}
	return map[string]any{"nf": nf, "path": f.Path, "yaml": f.Raw, "error": f.Err, "summary": sum}
}

// ---------- 审计 ----------
type auditItem struct {
	Level  string `json:"level"`
	Group  string `json:"group"`
	Detail string `json:"detail"`
}

func (c *Collector) Audit() map[string]any {
	cs := c.Configs()
	var items []auditItem
	add := func(l, g, d string) {
		items = append(items, auditItem{l, g, d})
	}
	summaries := map[string]map[string]any{}
	for nf, f := range cs.Items {
		if f.Data != nil {
			summaries[nf] = summarize(nf, f.Data)
		}
	}
	profs, mode := c.instances()
	registered := map[string]bool{}
	for _, p := range profs {
		registered[strings.ToLower(str(p["nfType"]))] = true
	}
	add("info", "通用", fmt.Sprintf("NRF 地址 <code>%s</code>；与配置目录 <code>nrfUri</code> 不一致会导致网元注册失联（mode=%s）", c.opt.NRFUri, mode))

	// 1 文件完整性
	var miss []string
	for _, n := range coreNFs {
		if _, ok := cs.Items[n]; !ok || summaries[n] == nil {
			miss = append(miss, n)
		}
	}
	if len(miss) > 0 {
		add("warn", "文件完整性", "缺失/不可解析："+strings.Join(miss, ", "))
	} else {
		add("ok", "文件完整性", fmt.Sprintf("核心 %d 个 NF 配置齐备（目录 %s）", len(coreNFs), coalesce(cs.Dir, "内置示例")))
	}
	// 2 版本强校验
	var bad []string
	nfOrder := make([]string, 0, len(summaries))
	for nf := range summaries {
		nfOrder = append(nfOrder, nf)
	}
	sort.Strings(nfOrder)
	for _, nf := range nfOrder {
		ev := expectVersion[nf]
		av := asString(summaries[nf]["version"])
		if ev != "" && av != ev {
			bad = append(bad, fmt.Sprintf("%s: 期望 %s，实际 %q", nf, ev, av))
		} else if ev != "" {
			add("ok", "版本号强校验", strings.ToLower(nf)+"cfg.yaml Info.Version = "+av+"（govalidator 白名单通过）")
		}
	}
	if len(bad) > 0 {
		add("error", "版本号强校验", strings.Join(bad, "；")+
			"。free5GC 各 NF 对版本号是 in(x.y.z) <b>强校验</b>，不匹配直接启动失败")
	}
	// 3 nrfUri
	for _, nf := range nfOrder {
		u := asString(summaries[nf]["nrfUri"])
		if u != "" && strings.TrimRight(u, "/") != strings.TrimRight(c.opt.NRFUri, "/") {
			add("warn", "NRF 地址一致", fmt.Sprintf("%s 的 nrfUri=<code>%s</code> 与网管使用的 <code>%s</code> 不一致", nf, u, c.opt.NRFUri))
		}
	}
	// 4 注册状态
	for _, nf := range nfOrder {
		if !registerNRF[nf] {
			continue
		}
		if !registered[nf] {
			add("warn", "网元注册", strings.ToUpper(nf)+" 有配置但未出现在 NRF 注册表 → 进程未启动或注册失败")
		} else {
			add("ok", "网元注册", strings.ToUpper(nf)+" 已在 NRF 注册")
		}
	}
	// 5 SMF upNodes.nodeID vs UPF pfcp.addr
	smf := summaries["smf"]
	upf := summaries["upf"]
	upfAddr := ""
	if upf != nil {
		upfAddr = asString(dig(upf["PFCP"], "nodeID"))
		if x := asString(dig(upf["PFCP"], "addr")); x != "" {
			upfAddr = x
		}
	}
	if smf != nil {
		for _, u := range listOr(smf["upNodes"]) {
			m := mapOf(u)
			nid := asString(m["nodeID"])
			if str(m["type"]) == "UPF" && nid != "" && upfAddr != "" {
				if nid != upfAddr {
					add("error", "SMF↔UPF", fmt.Sprintf("smfcfg.upNodes[%s].nodeID=%s 与 upfcfg PFCP 监听=%s 不一致 → PFCP 关联必败", asString(m["name"]), nid, upfAddr))
				} else {
					add("ok", "SMF↔UPF", asString(m["name"])+" nodeID="+nid+" 与 UPF PFCP 监听一致")
				}
			}
		}
	}
	// 6 DNN 集合
	smfDnn := map[string]bool{}
	if smf != nil {
		for _, x := range listOr(smf["snssaiDnn"]) {
			for _, d := range listOr(mapOf(x)["dnn"]) {
				smfDnn[strings.Trim(asString(d), "\"")] = true
			}
		}
	}
	upfDnn := map[string]bool{}
	if upf != nil {
		for _, d := range listOr(upf["DNN"]) {
			if s, ok := d.(string); ok {
				upfDnn[s] = true
			}
		}
	}
	amfDnn := map[string]bool{}
	if amf := summaries["amf"]; amf != nil {
		for _, d := range listOr(amf["dnn"]) {
			s, _ := d.(string)
			amfDnn[s] = true
		}
	}
	for _, pair := range []struct {
		tag  string
		a, b map[string]bool
	}{{"SMF vs UPF", smfDnn, upfDnn}, {"AMF vs SMF", amfDnn, smfDnn}} {
		if len(pair.a) == 0 && len(pair.b) == 0 {
			continue
		}
		var oa, ob []string
		for k := range pair.a {
			if !pair.b[k] {
				oa = append(oa, k)
			}
		}
		for k := range pair.b {
			if !pair.a[k] {
				ob = append(ob, k)
			}
		}
		if len(oa) == 0 && len(ob) == 0 {
			add("ok", "DNN 一致("+pair.tag+")", joinKeys(pair.a))
		} else {
			add("warn", "DNN 一致("+pair.tag+")", fmt.Sprintf("仅前者含 %v；仅后者含 %v", sorted(oa), sorted(ob)))
		}
	}
	// 7 UE 地址池 vs UPF cidr
	net2 := func(c string) string { return strings.Join(strings.Split(strings.Split(c, "/")[0], ".")[:2], ".") }
	var upfCidrs []string
	if upf != nil {
		cnt := map[string]bool{}
		walkCidrs(upf["dnnList"], cnt)
		upfCidrs = boolKeys(cnt)
	}
	for _, cidr := range upfCidrs {
		add("ok", "UPF 本地网关", "upfcfg dnn_list cidr="+cidr)
	}
	if smf != nil {
		for _, u := range listOr(smf["upNodes"]) {
			m := mapOf(u)
			if str(m["type"]) != "UPF" {
				continue
			}
			for _, pool := range boolKeys(strSet(m["cidrs"])) {
				hit := false
				for _, u := range upfCidrs {
					if net2(u) == net2(pool) {
						hit = true
						break
					}
				}
				if len(upfCidrs) > 0 && !hit {
					add("warn", "UE IP 池", fmt.Sprintf("smfcfg UPF pool <code>%s</code> 与 upfcfg cidr %s 不同网段。"+
						"free5GC <b>官方样例默认即 10.60.0.0/16 vs upf 60.60.0.0/24 不一致</b>：UE 可取址，"+
						"但 N6 回程按 UPF 子网路由会黑洞，把 smfcfg pools 改成 UPF cidr 同段", pool, upfCidrs))
				} else if len(upfCidrs) > 0 {
					add("ok", "UE IP 池", pool+" ⊂ "+strings.Join(upfCidrs, ","))
				}
			}
		}
	}
	// 8 N4 运行态
	st := false
	if upi := c.Userplane(); upi != nil {
		for _, v := range mapOf(upi["upNodes"]) {
			m, _ := v.(map[string]any)
			if m != nil && str(m["type"]) == "UPF" && dig(m, "status", "pfcpAssociationSetup") == true {
				st = true
			}
		}
	}
	if st {
		add("ok", "N4", "SMF UserPlaneInfo 显示存在已建立的 PFCP 关联")
	} else {
		add("warn", "N4", "未观察到已建立的 PFCP 关联（UPF 未起 / N4 断）")
	}
	// 9 OAuth/明文提示
	for _, nf := range nfOrder {
		if summaries[nf]["oauth"] == true {
			add("info", "OAuth", strings.ToUpper(nf)+" 配置开启 oauth —— 网管与 NF 间需带 bearer token")
		}
		if u := asString(summaries[nf]["nrfUri"]); strings.HasPrefix(u, "http://") {
			add("info", "安全", strings.ToUpper(nf)+" nrfUri 为明文 http（演示默认；生产建议 https+证书）")
			break
		}
	}
	rank := map[string]int{"error": 0, "warn": 1, "info": 2, "ok": 3}
	sort.SliceStable(items, func(i, j int) bool {
		return rank[items[i].Level] < rank[items[j].Level]
	})
	warn, errC := 0, 0
	for _, i := range items {
		if i.Level == "warn" {
			warn++
		}
		if i.Level == "error" {
			errC++
		}
	}
	c.emit("info", "审计", fmt.Sprintf("完成 %d 项（warn=%d error=%d） 用时 %s", len(items), warn, errC, "—"))
	return map[string]any{"items": items, "at": time.Now().Format("15:04:05")}
}

func strSet(v any) map[string]bool {
	out := map[string]bool{}
	switch s := v.(type) {
	case []string:
		for _, x := range s {
			out[x] = true
		}
	case []any:
		for _, x := range s {
			if str, ok := x.(string); ok && str != "" {
				out[str] = true
			}
		}
	}
	return out
}

func sorted(in []string) []string {
	cp := append([]string{}, in...)
	sort.Strings(cp)
	return cp
}
func joinKeys(m map[string]bool) string {
	var o []string
	for k := range m {
		o = append(o, k)
	}
	return strings.Join(sorted(o), ", ")
}
func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[2:])
		}
	}
	return p
}
