package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"ai-sign-in-gateway/internal/config"
	"ai-sign-in-gateway/internal/database"
	"ai-sign-in-gateway/internal/handlers"
	"ai-sign-in-gateway/internal/migrations"
	"ai-sign-in-gateway/internal/runtimecontrol"
	"ai-sign-in-gateway/internal/seed"
	"ai-sign-in-gateway/internal/services"
)

const appName = "ai-sign-in-gateway"
const (
	defaultBackendPort   = 8972
	defaultFrontendPort  = 3721
	maxDesktopPortOffset = 20
	runtimeProtocol      = 3
)

var (
	defaultHost        = "127.0.0.1"
	defaultOpenBrowser = "true"
)

type ShellConfig struct {
	Host               string `json:"host"`
	Port               int    `json:"port"`
	OpenBrowserOnStart bool   `json:"open_browser_on_start"`
	DataDir            string `json:"data_dir"`
}

type startupOptions struct {
	Host         string
	Port         int
	BackendPort  int
	FrontendPort int
	ConfigDir    string
	OpenBrowser  *bool
	Desktop      *bool
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("%s 启动失败: %v", appName, err)
	}
}

func run() error {
	opts, err := parseStartupOptions(os.Args[1:], os.Stdout)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	applyStartupOptions(opts)

	if targetURL := desktopWindowURL(); targetURL != "" {
		return runDesktopWindow(targetURL)
	}

	configDir, err := config.UserConfigDir()
	if err != nil {
		return err
	}
	defaultConfigDir, err := config.DefaultConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(configDir, "logs"), 0o755); err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(configDir, "logs", "shell.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		defer logFile.Close()
		log.SetOutput(io.MultiWriter(os.Stdout, logFile))
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	host := envString("AI_SIGN_IN_GATEWAY_HOST", defaultHost)
	backendPort := envFirstInt([]string{"AI_SIGN_IN_GATEWAY_BACKEND_PORT", "AI_SIGN_IN_GATEWAY_PORT"}, defaultBackendPort)
	frontendPort := envInt("AI_SIGN_IN_GATEWAY_FRONTEND_PORT", defaultFrontendPort)

	if desktopShellAvailable() && envBool("AI_SIGN_IN_GATEWAY_DESKTOP", true) {
		if existing, ok := findRunningDesktopService(host, frontendPort, maxDesktopPortOffset); ok {
			if existing.RuntimeProtocol < runtimeProtocol || runningServiceUsesDifferentConfig(existing, configDir) {
				if existing.RuntimeProtocol < runtimeProtocol {
					log.Printf("检测到旧版本本地服务，准备停止默认端口后启动新版: %s", existing.PublicURL)
				} else {
					log.Printf("检测到已运行服务使用不同配置目录，准备重启服务: 当前=%s 已运行=%s", configDir, existing.ConfigDir)
				}
				results := runtimecontrol.StopAppProcessesOnPorts([]int{
					frontendPort,
					backendPort,
					existing.Port,
					existing.BackendPort,
				}, nil, appName)
				for _, result := range results {
					log.Printf("停止旧版本端口 %d: %s", result.Port, result.Message)
				}
			} else {
				log.Printf("检测到已运行的本地服务，直接进入桌面窗口: %s", existing.PublicURL)
				return runDesktopWindow(desktopConsoleURL(existing.PublicURL))
			}
		}
	}

	backendLn, actualBackendPort, err := listenWithPortOffset(host, backendPort, maxDesktopPortOffset)
	if err != nil {
		return err
	}
	defer func() {
		_ = backendLn.Close()
	}()
	if actualBackendPort != backendPort {
		log.Printf("后端端口 %d 已占用，已自动切换到 %d", backendPort, actualBackendPort)
	}

	db, err := database.Open(cfg)
	if err != nil {
		return err
	}
	defer database.Close(db)
	if err := migrations.Apply(db); err != nil {
		return err
	}
	if err := seed.InitialData(db, cfg); err != nil {
		return err
	}

	api := handlers.NewRouter(db, cfg)
	backendURL := browserURL(host, actualBackendPort)
	gatewayURL := backendURL + "/api/gateway"

	if desktopShellAvailable() && envBool("AI_SIGN_IN_GATEWAY_DESKTOP", true) {
		frontendLn, actualFrontendPort, frontendErr := listenWithPortOffset(host, frontendPort, maxDesktopPortOffset)
		if frontendErr != nil {
			return frontendErr
		}
		defer func() {
			_ = frontendLn.Close()
		}()
		if actualFrontendPort != frontendPort {
			log.Printf("前端端口 %d 已占用，已自动切换到 %d", frontendPort, actualFrontendPort)
		}
		frontendURL := browserURL(host, actualFrontendPort)
		handlers.SetRuntimeInfo(handlers.RuntimeInfo{
			FrontendURL:                 frontendURL,
			FrontendDefaultPort:         frontendPort,
			FrontendPort:                actualFrontendPort,
			FrontendDefaultPortOccupant: defaultPortOwner(host, frontendPort, actualFrontendPort),
			BackendURL:                  backendURL,
			BackendDefaultPort:          backendPort,
			BackendPort:                 actualBackendPort,
			BackendDefaultPortOccupant:  defaultPortOwner(host, backendPort, actualBackendPort),
			GatewayURL:                  gatewayURL,
			RuntimeProtocol:             runtimeProtocol,
			ConfigDir:                   configDir,
			DefaultConfigDir:            defaultConfigDir,
			DatabasePath:                cfg.SQLitePath(),
		})

		backendServer := &http.Server{
			Addr:              net.JoinHostPort(host, strconv.Itoa(actualBackendPort)),
			Handler:           api,
			ReadHeaderTimeout: 15 * time.Second,
		}
		frontendServer := &http.Server{
			Addr:              net.JoinHostPort(host, strconv.Itoa(actualFrontendPort)),
			Handler:           desktopFrontendHandler(api, backendURL),
			ReadHeaderTimeout: 15 * time.Second,
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		go services.RunDatabaseBackupLoop(ctx, cfg.SQLitePath())
		log.Printf("%s 前端正在监听 %s", appName, frontendURL)
		log.Printf("%s 后端正在监听 %s", appName, backendURL)
		log.Printf("网关请求地址: %s", gatewayURL)
		log.Printf("用户配置目录: %s", configDir)
		return runDesktopShell(ctx, desktopRuntime{
			FrontendURL: frontendURL,
			BackendURL:  backendURL,
			GatewayURL:  gatewayURL,
			ConfigDir:   configDir,
			DB:          db,
			Backend:     backendServer,
			Frontend:    frontendServer,
			BackendLn:   backendLn,
			FrontendLn:  frontendLn,
		})
	}

	handlers.SetRuntimeInfo(handlers.RuntimeInfo{
		FrontendURL:                 backendURL,
		FrontendDefaultPort:         frontendPort,
		FrontendPort:                actualBackendPort,
		FrontendDefaultPortOccupant: defaultPortOwner(host, frontendPort, actualBackendPort),
		BackendURL:                  backendURL,
		BackendDefaultPort:          backendPort,
		BackendPort:                 actualBackendPort,
		BackendDefaultPortOccupant:  defaultPortOwner(host, backendPort, actualBackendPort),
		GatewayURL:                  gatewayURL,
		RuntimeProtocol:             runtimeProtocol,
		ConfigDir:                   configDir,
		DefaultConfigDir:            defaultConfigDir,
		DatabasePath:                cfg.SQLitePath(),
	})
	server := &http.Server{
		Addr:              net.JoinHostPort(host, strconv.Itoa(actualBackendPort)),
		Handler:           shellHandler(api),
		ReadHeaderTimeout: 15 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go services.RunDatabaseBackupLoop(ctx, cfg.SQLitePath())
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	log.Printf("%s Go 后端正在监听 %s", appName, backendURL)
	log.Printf("网关请求地址: %s", gatewayURL)
	log.Printf("用户配置目录: %s", configDir)
	if envBool("AI_SIGN_IN_GATEWAY_OPEN_BROWSER", defaultOpenBrowserEnabled()) {
		go openBrowser(backendURL)
	}

	err = server.Serve(backendLn)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func parseStartupOptions(args []string, output io.Writer) (startupOptions, error) {
	var opts startupOptions
	var shortPort int
	var openBrowser bool
	var noBrowser bool
	var desktop bool
	var noDesktop bool

	fs := flag.NewFlagSet(appName, flag.ContinueOnError)
	if output == nil {
		output = io.Discard
	}
	fs.SetOutput(output)
	fs.StringVar(&opts.Host, "host", "", "监听地址，例如 127.0.0.1 或 0.0.0.0")
	fs.IntVar(&opts.Port, "port", 0, "快速设置服务/API/网关端口")
	fs.IntVar(&shortPort, "p", 0, "快速设置服务/API/网关端口，等同 --port")
	fs.IntVar(&opts.BackendPort, "backend-port", 0, "桌面模式后端/API/网关端口，优先级高于 --port")
	fs.IntVar(&opts.FrontendPort, "frontend-port", 0, "桌面模式前端窗口入口端口")
	fs.StringVar(&opts.ConfigDir, "config-dir", "", "用户配置和数据库目录")
	fs.BoolVar(&openBrowser, "browser", false, "启动后打开浏览器或桌面窗口")
	fs.BoolVar(&noBrowser, "no-browser", false, "启动后不打开浏览器或桌面窗口")
	fs.BoolVar(&desktop, "desktop", false, "启用桌面 WebView/托盘")
	fs.BoolVar(&noDesktop, "no-desktop", false, "禁用桌面 WebView/托盘，仅作为本地 Web 服务运行")
	fs.Usage = func() {
		fmt.Fprintf(output, "%s 单文件快速运行:\n\n", appName)
		fmt.Fprintf(output, "  %s --port 9000\n", appName)
		fmt.Fprintf(output, "  %s --host 0.0.0.0 --port 9000 --no-browser\n", appName)
		fmt.Fprintf(output, "  %s --frontend-port 3722 --backend-port 8973\n\n", appName)
		fmt.Fprintln(output, "可用参数:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return startupOptions{}, err
	}
	if opts.Port > 0 && shortPort > 0 && opts.Port != shortPort {
		return startupOptions{}, fmt.Errorf("--port 和 -p 不能同时设置为不同端口")
	}
	if opts.Port == 0 {
		opts.Port = shortPort
	}
	if opts.BackendPort == 0 {
		opts.BackendPort = opts.Port
	}
	for label, port := range map[string]int{
		"--port":          opts.Port,
		"--backend-port":  opts.BackendPort,
		"--frontend-port": opts.FrontendPort,
	} {
		if port != 0 && !validPort(port) {
			return startupOptions{}, fmt.Errorf("%s 端口无效: %d", label, port)
		}
	}
	if openBrowser && noBrowser {
		return startupOptions{}, fmt.Errorf("--browser 和 --no-browser 不能同时使用")
	}
	if desktop && noDesktop {
		return startupOptions{}, fmt.Errorf("--desktop 和 --no-desktop 不能同时使用")
	}
	switch {
	case openBrowser:
		value := true
		opts.OpenBrowser = &value
	case noBrowser:
		value := false
		opts.OpenBrowser = &value
	}
	switch {
	case desktop:
		value := true
		opts.Desktop = &value
	case noDesktop:
		value := false
		opts.Desktop = &value
	}
	return opts, nil
}

func applyStartupOptions(opts startupOptions) {
	setEnvIfNotEmpty("AI_SIGN_IN_GATEWAY_HOST", opts.Host)
	setEnvIfNotEmpty("AI_SIGN_IN_GATEWAY_CONFIG_DIR", opts.ConfigDir)
	if opts.BackendPort > 0 {
		setEnvInt("AI_SIGN_IN_GATEWAY_BACKEND_PORT", opts.BackendPort)
		setEnvInt("AI_SIGN_IN_GATEWAY_PORT", opts.BackendPort)
	}
	if opts.FrontendPort > 0 {
		setEnvInt("AI_SIGN_IN_GATEWAY_FRONTEND_PORT", opts.FrontendPort)
	}
	if opts.OpenBrowser != nil {
		setEnvBool("AI_SIGN_IN_GATEWAY_OPEN_BROWSER", *opts.OpenBrowser)
	}
	if opts.Desktop != nil {
		setEnvBool("AI_SIGN_IN_GATEWAY_DESKTOP", *opts.Desktop)
	}
}

func validPort(port int) bool {
	return port > 0 && port <= 65535
}

func setEnvIfNotEmpty(key, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		_ = os.Setenv(key, value)
	}
}

func setEnvInt(key string, value int) {
	_ = os.Setenv(key, strconv.Itoa(value))
}

func setEnvBool(key string, value bool) {
	_ = os.Setenv(key, strconv.FormatBool(value))
}

func desktopFrontendHandler(api http.Handler, backendURL string) http.Handler {
	target, err := url.Parse(backendURL)
	if err != nil {
		return shellHandler(api)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	return funcHandler(func(w http.ResponseWriter, r *http.Request) {
		if shellAPIPath(r.URL.Path) {
			proxy.ServeHTTP(w, r)
			return
		}
		if embedded, ok := embeddedFrontend(); ok {
			serveEmbeddedFrontend(embedded, w, r)
			return
		}
		serveFrontend(findFrontendDist(), w, r)
	})
}

type funcHandler func(http.ResponseWriter, *http.Request)

func (fn funcHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fn(w, r)
}

func shellHandler(api http.Handler) http.Handler {
	if embedded, ok := embeddedFrontend(); ok {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if shellAPIPath(r.URL.Path) {
				api.ServeHTTP(w, r)
				return
			}
			serveEmbeddedFrontend(embedded, w, r)
		})
	}
	dist := findFrontendDist()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if shellAPIPath(r.URL.Path) {
			api.ServeHTTP(w, r)
			return
		}
		serveFrontend(dist, w, r)
	})
}

func shellAPIPath(path string) bool {
	return path == "/api" ||
		strings.HasPrefix(path, "/api/") ||
		path == "/v1" ||
		strings.HasPrefix(path, "/v1/")
}

func findFrontendDist() string {
	candidates := []string{}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "frontend", "dist"))
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(dir, "frontend", "dist"))
		candidates = append(candidates, filepath.Join(dir, "..", "frontend", "dist"))
	}
	for _, candidate := range candidates {
		if exists(filepath.Join(candidate, "index.html")) {
			return candidate
		}
	}
	return filepath.Join("frontend", "dist")
}

func pathWithinDir(root string, target string) bool {
	rootAbs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return false
	}
	targetAbs, err := filepath.Abs(filepath.Clean(target))
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

func serveFrontend(dist string, w http.ResponseWriter, r *http.Request) {
	if !exists(filepath.Join(dist, "index.html")) {
		http.Error(w, "frontend/dist 不存在，请先执行 npm run build", http.StatusServiceUnavailable)
		return
	}
	cleanPath := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if cleanPath == "." {
		cleanPath = "index.html"
	}
	requested := filepath.Join(dist, cleanPath)
	if pathWithinDir(dist, requested) {
		if info, err := os.Stat(requested); err == nil && !info.IsDir() {
			if contentType := mime.TypeByExtension(filepath.Ext(requested)); contentType != "" {
				w.Header().Set("Content-Type", contentType)
			}
			http.ServeFile(w, r, requested)
			return
		}
	}
	if isStaticAssetRequest(r.URL.Path) {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join(dist, "index.html"))
}

func serveEmbeddedFrontend(frontend fs.FS, w http.ResponseWriter, r *http.Request) {
	cleanPath := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if cleanPath == "." || cleanPath == "/" {
		cleanPath = "index.html"
	}
	cleanPath = filepath.ToSlash(cleanPath)
	if strings.HasPrefix(cleanPath, "../") || cleanPath == ".." {
		http.NotFound(w, r)
		return
	}
	if info, err := fs.Stat(frontend, cleanPath); err == nil && !info.IsDir() {
		if contentType := mime.TypeByExtension(filepath.Ext(cleanPath)); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		http.ServeContent(w, r, cleanPath, info.ModTime(), mustOpenEmbedded(frontend, cleanPath))
		return
	}
	if isStaticAssetRequest(r.URL.Path) {
		http.NotFound(w, r)
		return
	}
	index, err := fs.ReadFile(frontend, "index.html")
	if err != nil {
		http.Error(w, "embedded frontend/index.html 不存在", http.StatusServiceUnavailable)
		return
	}
	if contentType := mime.TypeByExtension(".html"); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
}

func mustOpenEmbedded(frontend fs.FS, name string) *bytes.Reader {
	content, err := fs.ReadFile(frontend, name)
	if err != nil {
		panic("embedded frontend asset is not readable: " + name)
	}
	return bytes.NewReader(content)
}

func isStaticAssetRequest(requestPath string) bool {
	cleanPath := strings.TrimSpace(requestPath)
	if cleanPath == "" || cleanPath == "/" {
		return false
	}
	if strings.HasPrefix(cleanPath, "/assets/") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(cleanPath))
	switch ext {
	case ".js", ".mjs", ".css", ".map", ".json", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico", ".woff", ".woff2", ".ttf", ".otf", ".txt", ".xml":
		return true
	default:
		return false
	}
}

func browserURL(host string, port int) string {
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s", net.JoinHostPort(host, strconv.Itoa(port)))
}

func openBrowser(target string) {
	time.Sleep(500 * time.Millisecond)
	_ = runtimecontrol.OpenURL(target)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func envString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envFirstInt(keys []string, fallback int) int {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func defaultPortOwner(host string, defaultPort, currentPort int) string {
	if defaultPort == currentPort {
		return "当前程序"
	}
	if !runtimecontrol.IsPortOccupied(localProbeHost(host), defaultPort) {
		return "未占用"
	}
	return describePortOccupant(defaultPort)
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func defaultOpenBrowserEnabled() bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(defaultOpenBrowser))
	if err != nil {
		return true
	}
	return parsed
}
