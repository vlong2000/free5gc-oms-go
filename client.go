package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// sbiGet 请求 free5GC NF 的 SBI 接口 → (status, body, err)。
// free5GC 的 util/httpwrapper 同时接受 HTTP/1.1 与 h2c（官方 curl 示例即 h1），
// 标准库 h1 客户端即可；TLS 自签证书跳过校验（与 webconsole httpsClient 同策）。
func sbiGet(url string, timeout time.Duration) (int, []byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: timeout, Transport: defaultTLS()}).Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return resp.StatusCode, body, nil
}

// sbiPost JSON body POST，返回 status/body/err。
func sbiPost(url string, payload any, timeout time.Duration) (int, []byte, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: timeout, Transport: defaultTLS()}).Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, body, nil
}

// sbiSelfTest GET 服务前缀；gin 未命中路由返回 404 也代表存活（同 webconsole 探活观感）。
func sbiSelfTest(url string) map[string]any {
	t0 := time.Now()
	st, _, err := sbiGet(url, 3*time.Second)
	ms := float64(time.Since(t0).Microseconds()) / 1000.0
	out := map[string]any{"ms": round1(ms)}
	if err != nil {
		out["ok"] = false
		out["error"] = fmt.Sprint(err)
		return out
	}
	out["ok"] = st < 500
	out["status"] = st
	return out
}

func tcpProbe(addr string, timeout time.Duration) (bool, float64) {
	t0 := time.Now()
	c, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false, 0
	}
	_ = c.Close()
	return true, round1(float64(time.Since(t0).Microseconds()) / 1000.0)
}

// serviceURI 复刻 webconsole/backend/webui_context/context.go 的 getNfOamUri：
// Fqdn → service.Fqdn → apiPrefix → ipEndPoints(ip:port, port=0 时按 scheme 补 80/443)。
// 输入为 NRF 返回的 NFProfile 原始 map。
func serviceURI(profile map[string]any, serviceName string) string {
	svcs, _ := profile["nfServices"].([]any)
	for _, s := range svcs {
		svc, _ := s.(map[string]any)
		if svc == nil {
			continue
		}
		if !svcHasName(svc, serviceName) {
			continue
		}
		if st := svc["nfServiceStatus"]; st != nil && st != "REGISTERED" {
			continue
		}
		api := "/" + serviceName + "/v1"
		scheme := "http"
		if vers, ok := svc["versions"].([]any); ok && len(vers) > 0 {
			if v, ok := vers[0].(map[string]any); ok {
				if p, ok := v["apiPrefix"].(string); ok && p != "" {
					api = p
				}
				if sc, ok := v["uriScheme"].(string); ok && sc != "" {
					scheme = sc
				}
			}
		}
		if fqdn, ok := svc["fqdn"].(string); ok && fqdn != "" {
			return scheme + "://" + fqdn + api
		}
		if eps, ok := svc["ipEndPoints"].([]any); ok && len(eps) > 0 {
			ep, _ := eps[0].(map[string]any)
			host, _ := ep["ipv4Address"].(string)
			if host == "" {
				host, _ = ep["ipv6Address"].(string)
			}
			if host == "" {
				host, _ = profile["fqdn"].(string)
			}
			if host == "" {
				continue
			}
			port := 0
			if n, ok := ep["port"].(float64); ok {
				port = int(n)
			}
			if port == 0 {
				if scheme == "https" {
					port = 443
				} else {
					port = 8000 // free5GC 缺省 SBI 端口
				}
			}
			return fmt.Sprintf("%s://%s:%d%s", scheme, host, port, api)
		}
	}
	return ""
}

// svcHasName：NRF profiles 里 servicenames 既可为字符串数组，也可能单值 serviceName。
func svcHasName(svc map[string]any, name string) bool {
	if ns, ok := svc["servicenames"].([]any); ok {
		for _, n := range ns {
			if s, _ := n.(string); s == name {
				return true
			}
		}
	}
	if s, _ := svc["serviceName"].(string); s == name {
		return true
	}
	return false
}

func round1(f float64) float64 { return float64(int64(f*10+0.5)) / 10 }

func jsonRaw(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
