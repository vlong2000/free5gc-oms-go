// Command free5gc-oms 是 free5GC（3GPP R17 SBA，Go 实现）的配套网管/监控系统。
//
// 设计与 free5GC 代码一一对应（读码依据见 README）：
//
//	NRF   GET  {nrf}/nnrf-nfm/v1/nf-instances          注册表（与 webconsole UpdateNfProfiles 同源）
//	NRF   GET  {nrf}/nnrf-disc/v1/nf-instances         发现视图（NFDiscoveryProcess）
//	AMF   GET  /namf-oam/v1/registered-ue-context      producer/oam.go OAMUeContext
//	SMF   GET  /nsmf-oam/v1/ue-pdu-session-info/{ref}  context.SMContext 的 OAM dump
//	SMF   GET  /nsmf-oam/v1/user-plane-info            UserPlaneInformation（含 N4 关联状态）
//	配置   直读 config/<nf>cfg.yaml（各 NF pkg/factory 的加载面）
//
// 前端 web/ 通过 go:embed 打包为单一二进制；NRF 不可达且 --mock=auto 时自动切演示数据。
package main

import (
	"context"
	"crypto/tls"
	"embed"
	"flag"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

//go:embed web
var webFS embed.FS

// Options 命令行/环境配置。
type Options struct {
	Port      int
	NRFUri    string
	ConfigDir string
	Mock      string // auto | live | force
	User      string
	Password  string
}

func main() {
	opt := Options{}
	flag.IntVar(&opt.Port, "port", 3000, "监听端口")
	flag.StringVar(&opt.NRFUri, "nrf", envOr("OMS_NRF", "http://127.0.0.10:8000"), "NRF SBI 地址")
	flag.StringVar(&opt.ConfigDir, "config-dir", envOr("OMS_CONFIG_DIR", ""), "free5GC config 目录（缺省用内置示例）")
	flag.StringVar(&opt.Mock, "mock", envOr("OMS_MOCK", "auto"), "auto|live|force 演示数据策略")
	flag.StringVar(&opt.User, "user", "", "Basic-Auth 用户（与 password 同给生效）")
	flag.StringVar(&opt.Password, "password", "", "Basic-Auth 口令")
	flag.Parse()
	if (opt.User == "") != (opt.Password == "") {
		log.Fatal("--user 与 --password 必须同时提供")
	}

	c := NewCollector(opt)
	mux := http.NewServeMux()
	c.Routes(mux)

	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "/index.html":
			http.ServeFileFS(w, r, sub, "index.html")
			return
		case "/static/app.js":
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			http.ServeFileFS(w, r, sub, "app.js")
			return
		case "/static/style.css":
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
			http.ServeFileFS(w, r, sub, "style.css")
			return
		}
		fileServer.ServeHTTP(w, r)
	}))

	var handler http.Handler = mux
	if opt.User != "" {
		handler = basicAuth(opt.User, opt.Password, mux)
	}
	srv := &http.Server{
		Addr:    net.JoinHostPort("0.0.0.0", strconv.Itoa(opt.Port)),
		Handler: handler,
	}
	go func() {
		log.Printf("[oms-go] http://0.0.0.0:%d  nrf=%s  config=%s  mock=%s  auth=%v",
			opt.Port, opt.NRFUri, coalesce(opt.ConfigDir, "<内置示例>"), opt.Mock, opt.User != "")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// DefaultTransport 跳过 free5GC 运行期自签证书校验（同 webconsole httpsClient），
// 超时统一 3s（官方 NF 多为本地回环，1s 足够，保守留出 TLS 头寸）。
func defaultTLS() http.RoundTripper {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // 网管与 NF 同机，见 README 安全说明
	t.ResponseHeaderTimeout = 5 * time.Second
	t.DisableKeepAlives = false
	return t
}

func basicAuth(user, pass string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if ok && u == user && p == pass {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="free5gc-oms"`)
		w.WriteHeader(http.StatusUnauthorized)
	})
}

func coalesce(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
