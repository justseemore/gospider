package browser

import (
	"archive/zip"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"gitee.com/baixudong/gospider/cdp"
	"gitee.com/baixudong/gospider/cmd"
	"gitee.com/baixudong/gospider/conf"
	"gitee.com/baixudong/gospider/proxy"
	"gitee.com/baixudong/gospider/re"
	"gitee.com/baixudong/gospider/requests"
	"gitee.com/baixudong/gospider/tools"
)

var version = "020"

func delTempDir(dir string) {
	for i := 0; i < 500; i++ {
		if os.RemoveAll(dir) == nil {
			return
		}
		time.Sleep(time.Millisecond * 10)
	}
}

// go build -ldflags="-H windowsgui" -o browser/browserCmd.exe main.go
// go build -o browser/browserCmd main.go
func BrowserCmdMain() {
	preCtx := context.Background()
	ctx, cnl := context.WithCancel(preCtx)
	pipData := make(chan struct{})
	data := map[string]any{}
	args := []string{}
	var cmdCli *cmd.Client
	go func() {
		defer cnl()
		jsonDecode := json.NewDecoder(os.Stdin)
		err := jsonDecode.Decode(&data)
		if err != nil || data["name"] == nil {
			return
		}
		jsonData := tools.Any2json(data)
		for _, arg := range jsonData.Get("args").Array() {
			args = append(args, arg.String())
		}
		cmdCli = cmd.NewClient(ctx, cmd.ClientOption{Name: jsonData.Get("name").String(), Args: args})
		go cmdCli.Run()
		pipData <- struct{}{}
		jsonDecode.Decode(&data)
	}()
	select {
	case <-ctx.Done():
		return
	case <-pipData:
	case <-time.After(time.Second * 2):
		return
	}
	//dong some thing
	for _, arg := range args {
		if strings.Contains(arg, "--user-data-dir=") {
			rs := re.Search(`--user-data-dir="(.*?)"`, arg)
			if rs != nil {
				defer delTempDir(rs.Group(1))
			} else {
				rs = re.Search(`--user-data-dir=(.*)`, arg)
				if rs != nil {
					defer delTempDir(rs.Group(1))
				}
			}
		}
	}
	//join
	defer cmdCli.Close()
	select {
	case <-ctx.Done():
	case <-cmdCli.Done():
	}
}

//go:embed stealth.min.js
var stealth string

//go:embed stealth2.min.js
var stealth2 string

type Client struct {
	db     *cdp.DbClient
	cmdCli *cmd.Client
	reqCli *requests.Client
	port   int
	host   string
	sync.Mutex
	ctx      context.Context
	cnl      context.CancelFunc
	webSock  *cdp.WebSock
	proxyCli *proxy.Client
	disRoute bool //关闭默认路由
	proxy    string
	getProxy func() (string, error)
}
type ClientOption struct {
	EvalPath  string   //浏览器执行路径
	Host      string   //连接host
	Port      int      //连接port
	UserDir   string   //设置用户目录
	Args      []string //启动参数
	Headless  bool     //是否使用无头
	UserAgent string
	Proxy     string                 //代理
	GetProxy  func() (string, error) //代理
	DisRoute  bool                   //关闭默认路由
}

//go:embed browserCmd.exe
var browserCmdWindows []byte

//go:embed browserCmd
var browserCmdLinux []byte

func getCmdName() (string, error) {
	mainDir, err := conf.GetMainDirPath()
	if err != nil {
		return "", err
	}
	fileName := tools.PathJoin(mainDir, fmt.Sprintf("browserCmd%s", version))
	if runtime.GOOS == "windows" {
		fileName += ".exe"
	}
	if !tools.PathExist(fileName) {
		os.MkdirAll(mainDir, 0777)
		if runtime.GOOS == "windows" {
			if err = os.WriteFile(fileName, browserCmdWindows, 0777); err != nil {
				return "", err
			}
		} else {
			if err = os.WriteFile(fileName, browserCmdLinux, 0777); err != nil {
				return "", err
			}
		}
	}
	return fileName, nil
}

type downClient struct {
	sync.Mutex
}

var oneDown = &downClient{}

func (obj *downClient) getChromePath(preCtx context.Context) (string, error) {
	obj.Lock()
	defer obj.Unlock()
	chromDir, err := conf.GetMainDirPath()
	if err != nil {
		return "", err
	}
	var chromePath string
	switch runtime.GOOS {
	case "windows":
		chromePath = tools.PathJoin(chromDir, "chrome-win", "chrome.exe")
	case "darwin":
		chromePath = tools.PathJoin(chromDir, "chrome-mac", "Chromium.app")
	case "linux":
		chromeArgs = append(chromeArgs,
			"--use-gl=swiftshader",
			"--disable-gpu",
		)
		chromePath = tools.PathJoin(chromDir, "chrome-linux", "chrome")
	default:
		return "", errors.New("dont know goos")
	}
	if !tools.PathExist(chromePath) {
		if err = downChrome(preCtx); err != nil {
			return "", err
		}
		if !tools.PathExist(chromePath) {
			return "", errors.New("not found chrome")
		}
	}
	return chromePath, nil
}

func clearTemp() {
	tempDir := os.TempDir()
	dirs, err := os.ReadDir(tempDir)
	if err != nil {
		return
	}
	for _, dir := range dirs {
		if re.Search(fmt.Sprintf(`%s\d+$`, conf.TempChromeDir), dir.Name()) != nil {
			os.RemoveAll(tools.PathJoin(tempDir, dir.Name()))
		}
	}
}
func runChrome(ctx context.Context, option *ClientOption) (*cmd.Client, error) {
	fileName, err := getCmdName()
	if err != nil {
		return nil, err
	}
	if option.Host == "" {
		option.Host = "127.0.0.1"
	}
	if option.Port == 0 {
		option.Port, err = tools.FreePort()
		if err != nil {
			return nil, err
		}
	}
	if option.UserAgent == "" {
		option.UserAgent = requests.UserAgent
	}
	if option.EvalPath == "" {
		option.EvalPath, err = oneDown.getChromePath(ctx)
		if err != nil {
			return nil, err
		}
	}
	if option.UserDir == "" {
		option.UserDir, err = os.MkdirTemp(os.TempDir(), conf.TempChromeDir)
		if err != nil {
			return nil, err
		}
	}

	cli := cmd.NewLeakClient(ctx, cmd.ClientOption{
		Name: fileName,
	})
	inP, err := cli.StdInPipe()
	if err != nil {
		return nil, err
	}
	args := []string{}
	args = append(args, chromeArgs...)
	if option.UserAgent != "" {
		args = append(args, fmt.Sprintf("--user-agent=%s", option.UserAgent))
	}
	if option.Headless {
		args = append(args, "--headless")
	}
	if option.Proxy != "" {
		args = append(args, fmt.Sprintf(`--proxy-server=%s`, option.Proxy))
	}
	args = append(args, fmt.Sprintf(`--user-data-dir=%s`, option.UserDir))
	args = append(args, fmt.Sprintf("--remote-debugging-port=%d", option.Port))
	args = append(args, option.Args...)
	_, err = inP.Write(tools.StringToBytes(tools.Any2json(map[string]any{
		"name": option.EvalPath,
		"args": args,
	}).Raw))
	if err != nil {
		return nil, err
	}
	go cli.Run()
	return cli, cli.Err
}

var chromeArgs = []string{
	"--useAutomationExtension=false",
	"--excludeSwitches=enable-automation",
	"--no-sandbox",
	"--no-pings",
	"--no-zygote",
	"--mute-audio",
	"--no-first-run",
	"--no-default-browser-check",
	"--disable-software-rasterizer",
	"--disable-cloud-import",
	"--disable-gesture-typing",
	"--disable-setuid-sandbox",
	"--disable-offer-store-unmasked-wallet-cards",
	"--disable-offer-upload-credit-cards",
	"--disable-print-preview",
	"--disable-voice-input",
	"--disable-wake-on-wifi",
	"--disable-cookie-encryption",
	"--ignore-gpu-blocklist",
	"--enable-async-dns",
	"--enable-simple-cache-backend",
	"--enable-tcp-fast-open",
	"--enable-webgl",
	"--prerender-from-omnibox=disabled",
	"--enable-web-bluetooth",
	"--disable-site-isolation-trials",
	"--disable-features=AudioServiceOutOfProcess,IsolateOrigins,site-per-process,TranslateUI,BlinkGenPropertyTrees", // do not disable UserAgentClientHint
	"--aggressive-cache-discard",
	"--disable-extensions",
	"--disable-ipc-flooding-protection",
	"--disable-default-apps",
	"--disable-blink-features=AutomationControlled",
	"--enable-features=NetworkService,NetworkServiceInProcess,TrustTokens,TrustTokensAlwaysAllowIssuance", // support ServiceWorkers
	"--disable-breakpad",
	"--disable-component-update",
	"--disable-domain-reliability",
	"--disable-sync",
	"--disable-client-side-phishing-detection",
	"--disable-hang-monitor",
	"--disable-popup-blocking",
	"--disable-crash-reporter",
	"--disable-dev-shm-usage",
	"--disable-background-networking",
	"--disable-background-timer-throttling",
	"--disable-backgrounding-occluded-windows",
	"--disable-infobars",
	"--hide-scrollbars",
	"--disable-prompt-on-repost",
	"--metrics-recording-only",
	"--safebrowsing-disable-auto-update",
	"--use-mock-keychain",
	"--force-webrtc-ip-handling-policy=default_public_interface_only",
	"--disable-session-crashed-bubble",
	"--disable-renderer-backgrounding",
	"--font-render-hinting=none",
	"--disable-logging",
	"--enable-surface-synchronization",
	"--run-all-compositor-stages-before-draw",
	"--disable-threaded-animation",
	"--disable-threaded-scrolling",
	"--disable-checker-imaging",
	"--blink-settings=primaryHoverType=2,availableHoverTypes=2,primaryPointerType=4,availablePointerTypes=4",
	"--blink-settings=imagesEnabled=true",
	"--ignore-ssl-errors=true",
	"--ssl-protocol=any",
	"--autoplay-policy=no-user-gesture-required",
	"--force-color-profile=srgb",
	"--disable-partial-raster",
	"--disable-component-extensions-with-background-pages",
	"--disable-new-content-rendering-timeout",
	"--disable-translate",
	"--password-store=basic",
	"--disable-image-animation-resync",
}

//go:embed devices.json
var devicesData []byte

var Devices = loadDevicesData()

func loadDevicesData() map[string]cdp.Device {
	var result map[string]cdp.Device
	if err := json.Unmarshal(devicesData, &result); err != nil {
		log.Panic(err)
	}
	return result
}
func downChromeFile(preCtx context.Context, dirUrl string) error {
	reqCli, err := requests.NewClient(preCtx)
	if err != nil {
		return err
	}
	resp, err := reqCli.Request(preCtx, "get", dirUrl)
	if err != nil {
		return err
	}
	var fileDir string
	var fileTime int64
	for _, dir := range resp.Json().Array() {
		tempTime, err := time.Parse(fmt.Sprintf("%sT%sZ", time.DateOnly, time.TimeOnly), dir.Get("date").String())
		if err == nil && tempTime.Unix() > fileTime {
			fileDir = dir.Get("url").String()
			fileTime = tempTime.Unix()
		}
	}
	if fileTime == 0 {
		return errors.New("not found chrome dir")
	}
	resp, err = reqCli.Request(preCtx, "get", fileDir)
	if err != nil {
		return err
	}
	fileUrl := resp.Json().Get("0.url").String()
	resp, err = reqCli.Request(preCtx, "get", fileUrl, requests.RequestOption{Bar: true})
	if err != nil {
		return err
	}
	zipData, err := zip.NewReader(bytes.NewReader(resp.Content()), int64(len(resp.Content())))
	if err != nil {
		return err
	}
	mainDir, err := conf.GetMainDirPath()
	if err != nil {
		return err
	}
	for _, file := range zipData.File {
		filePath := tools.PathJoin(mainDir, file.Name)
		fileDirPath := tools.PathJoin(filePath, "..")
		if !tools.PathExist(fileDirPath) {
			if err = os.MkdirAll(fileDirPath, 0777); err != nil {
				return err
			}
		}
		readBody, err := file.Open()
		if err != nil {
			return err
		}
		tempBody := bytes.NewBuffer(nil)
		if _, err = io.Copy(tempBody, readBody); err != nil {
			return err
		}
		if err = os.WriteFile(filePath, tempBody.Bytes(), 0777); err != nil {
			return err
		}
	}
	return err
}
func downChrome(preCtx context.Context) error {
	switch runtime.GOOS {
	case "windows":
		return downChromeFile(preCtx, "https://registry.npmmirror.com/-/binary/chromium-browser-snapshots/Win_x64/")
	case "darwin":
		return downChromeFile(preCtx, "https://registry.npmmirror.com/-/binary/chromium-browser-snapshots/Mac/")
	case "linux":
		return downChromeFile(preCtx, "https://registry.npmmirror.com/-/binary/chromium-browser-snapshots/Linux_x64/")
	default:
		return errors.New("dont know goos")
	}
}

// 新建浏览器
func NewClient(preCtx context.Context, options ...ClientOption) (*Client, error) {
	clearTemp()
	var option ClientOption
	if len(options) > 0 {
		option = options[0]
	}
	if preCtx == nil {
		preCtx = context.TODO()
	}
	ctx, cnl := context.WithCancel(preCtx)
	var cli *cmd.Client
	var err error
	if option.Host == "" || option.Port == 0 {
		cli, err = runChrome(ctx, &option)
	}
	if err != nil {
		cnl()
		return nil, err
	}
	reqCli, err := requests.NewClient(ctx)
	if err != nil {
		cnl()
		return nil, err
	}

	db, err := cdp.NewDbClient(option.UserDir)
	if err != nil {
		cnl()
		return nil, err
	}
	client := &Client{
		proxy:    option.Proxy,
		getProxy: option.GetProxy,
		ctx:      ctx,
		cnl:      cnl,
		cmdCli:   cli,
		db:       db,
		host:     option.Host,
		port:     option.Port,
		reqCli:   reqCli,
		disRoute: option.DisRoute,
	}
	return client, client.init()
}

// 浏览器初始化
func (obj *Client) init() error {
	nowTime := time.Now().Unix()
	rs, err := obj.reqCli.Request(obj.ctx, "get",
		fmt.Sprintf("http://%s:%d/json/version", obj.host, obj.port),
		requests.RequestOption{
			AfterCallBack: func(ro *requests.RequestOption, r *requests.Response) *requests.Response {
				if ro.Err != nil || r.StatusCode() != 200 {
					ro.Err = errors.New("code error")
					if time.Now().Unix()-nowTime < 10 {
						time.Sleep(time.Millisecond * 1000)
					} else {
						ro.CurTryNum = 10
					}
				}
				return r
			},
			TryNum: 10,
		})
	if err != nil {
		if obj.cmdCli.Err != nil {
			return obj.cmdCli.Err
		}
		return err
	}
	wsUrl := rs.Json().Get("webSocketDebuggerUrl").String()
	if wsUrl == "" {
		return errors.New("not fouond browser wsUrl")
	}
	browWsRs := re.Search(`devtools/browser/(.*)`, wsUrl)
	if browWsRs == nil {
		return errors.New("not fouond browser id")
	}
	obj.webSock, err = cdp.NewWebSock(
		obj.ctx,
		fmt.Sprintf("ws://%s:%d/devtools/browser/%s", obj.host, obj.port, browWsRs.Group(1)),
		fmt.Sprintf("http://%s:%d", obj.host, obj.port),
		"",
		nil,
		obj.db,
	)
	if err != nil {
		return err
	}
	obj.proxyCli, err = proxy.NewClient(obj.ctx, proxy.ClientOption{
		Host: tools.GetHost(),
		Port: obj.port,
	})
	if err != nil {
		return err
	}
	obj.proxyCli.DisVerify = true
	obj.proxyCli.Proxy = fmt.Sprintf("http://%s:%d", obj.host, obj.port)
	go obj.proxyCli.Run()
	return obj.proxyCli.Err
}
func (obj *Client) Done() <-chan struct{} {
	return obj.webSock.Done()
}
func (obj *Client) Addr() string {
	return obj.proxyCli.Addr()
}
func (obj *Client) Close() error {
	if obj.webSock != nil {
		if err := obj.webSock.BrowserClose(); err != nil {
			return err
		}
	}
	if obj.cmdCli != nil {
		obj.cmdCli.Close()
	}
	obj.cnl()
	return obj.db.Close()
}

// 新建标签页
func (obj *Client) NewPage(preCtx context.Context, options ...PageOption) (*Page, error) {
	var option PageOption
	if len(options) > 0 {
		option = options[0]
	}
	if option.Proxy == "" {
		option.Proxy = obj.proxy
	}
	if option.GetProxy == nil {
		option.GetProxy = obj.getProxy
	}

	rs, err := obj.webSock.TargetCreateTarget(preCtx, "")
	if err != nil {
		return nil, err
	}
	targetId, ok := rs.Result["targetId"].(string)
	if !ok {
		return nil, errors.New("not found targetId")
	}
	ctx, cnl := context.WithCancel(obj.ctx)
	page := &Page{
		id:         targetId,
		preWebSock: obj.webSock,
		port:       obj.port,
		host:       obj.host,
		ctx:        ctx,
		cnl:        cnl,
	}
	if err = page.init(option.Proxy, option.GetProxy, obj.db); err != nil {
		return nil, err
	}
	if !obj.disRoute {
		if err = page.Route(preCtx, func(ctx context.Context, r *cdp.Route) {
			rs, err := r.Request(ctx, r.NewRequestOption())
			if err != nil {
				r.Fail(ctx, "Failed")
			} else if err = r.FulFill(ctx, rs); err != nil {
				r.Fail(ctx, "Failed")
			}
		}); err != nil {
			return nil, err
		}
	}
	return page, nil
}
