package splash

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"gitee.com/baixudong/gospider/blog"
	"gitee.com/baixudong/gospider/browser"
	"gitee.com/baixudong/gospider/cdp"
	"gitee.com/baixudong/gospider/requests"
	"gitee.com/baixudong/gospider/thread"
	"gitee.com/baixudong/gospider/tools"

	"github.com/gin-gonic/gin"
)

type Client struct {
	addr       string
	proxy      string
	headless   bool
	tasks      chan *Task
	ctx        context.Context
	pageNum    int
	browserNum int
	logCli     *blog.Client
}
type ClientOption struct {
	Host     string
	Port     int
	Proxy    string
	Headless bool

	BrowserNum int
	PageNum    int
	logPath    string
}

func NewFlagClient(ctx context.Context) (*Client, error) {
	browserNum := flag.Int("browserNum", 1, "浏览器的数量")
	pageNum := flag.Int("pageNum", 100, "标签页最大打开数量")
	host := flag.String("host", "", "host")
	port := flag.Int("port", 8210, "port")
	proxy := flag.String("proxy", "", "代理")
	logPath := flag.String("log", "", "日志文件")
	headless := flag.Bool("headless", true, "是否开启无头模式")
	flag.Parse()
	return NewClient(ctx, ClientOption{
		Host:       *host,
		Port:       *port,
		Headless:   *headless,
		Proxy:      *proxy,
		BrowserNum: *browserNum,
		PageNum:    *pageNum,
		logPath:    *logPath,
	})
}
func NewClient(ctx context.Context, options ...ClientOption) (*Client, error) {
	var client = new(Client)
	if ctx == nil {
		ctx = context.TODO()
	}
	client.ctx = ctx
	var option ClientOption
	if len(options) > 0 {
		option = options[0]
	}
	if option.BrowserNum < 1 {
		option.BrowserNum = 1
	}
	if option.PageNum < 1 {
		option.PageNum = 100
	}
	client.pageNum = option.PageNum
	client.browserNum = option.BrowserNum
	client.tasks = make(chan *Task, option.BrowserNum*option.PageNum)

	if option.Host == "" {
		option.Host = "0.0.0.0"
	}
	if option.logPath == "" {
		option.logPath = "splash.log"
	}
	var err error
	if option.Port == 0 {
		if option.Port, err = tools.FreePort(); err != nil {
			return nil, err
		}
	}
	client.addr = net.JoinHostPort(option.Host, strconv.Itoa(option.Port))
	client.headless = option.Headless
	client.proxy = option.Proxy
	client.logCli = blog.NewClient(option.logPath)
	if option.Proxy != "" {
		href, err := url.Parse(option.Proxy)
		if err != nil {
			return nil, err
		}
		if href.Scheme != "http" && href.Scheme != "socks5" {
			return nil, errors.New("proxy scheme error")
		}
	}
	return client, nil
}

type Task struct {
	rendOption RendOption
	Task       *thread.Task
	ctx        context.Context
	cnl        context.CancelFunc
	reqCtx     context.Context
	err        error
}

func (obj *Task) result(preCtx context.Context) (taskResult, error) {
	var result taskResult
	select {
	case <-preCtx.Done():
		return result, errors.New("请求周期结束")
	case <-obj.Task.Done():
		if obj.Task.Error != nil {
			return result, obj.Task.Error
		}
		results := obj.Task.Result
		if len(results) != 2 {
			return result, errors.New("not found result")
		}
		if results[1] != nil {
			return result, results[1].(error)
		}
		result, ok := results[0].(taskResult)
		if !ok {
			return result, errors.New("parse result panic")
		}
		return result, nil
	}
}
func (obj *Client) write(preCtx context.Context, option RendOption) (*Task, error) {
	task := new(Task)
	task.rendOption = option
	task.ctx, task.cnl = context.WithCancel(preCtx)
	task.reqCtx = preCtx
	obj.tasks <- task
	select {
	case <-task.ctx.Done():
		return task, task.err
	case <-time.After(time.Second * 60):
		return task, errors.New("time out")
	}
}
func (obj *Client) error(ctx *gin.Context, url string, code int, err error) {
	obj.logCli.Info(code, map[string]any{"url": url, "err": err})
	ctx.Header("error", err.Error())
	ctx.String(code, "")
}

func (obj *Client) mainHandler(ctx *gin.Context, option RendOption) {
	href, err := url.Parse(option.Url)
	if err != nil {
		obj.error(ctx, option.Url, 503, err)
		return
	}
	if href.Scheme != "http" && href.Scheme != "https" {
		obj.error(ctx, option.Url, 504, errors.New("url scheme error"))
		return
	}
	if option.Proxy != "" {
		if href, err = url.Parse(option.Proxy); err != nil {
			obj.error(ctx, option.Url, 505, err)
			return
		}
		if href.Scheme != "http" && href.Scheme != "socks5" {
			obj.error(ctx, option.Url, 506, errors.New("proxy scheme error"))
			return
		}
	}
	option.cookies = ctx.Request.Cookies()
	task, err := obj.write(ctx.Request.Context(), option)
	if err != nil {
		obj.error(ctx, option.Url, 501, err)
		return
	}
	rs, err := task.result(task.reqCtx)
	if err != nil {
		obj.error(ctx, option.Url, 502, err)
		return
	}
	for _, cook := range rs.Cookies {
		http.SetCookie(ctx.Writer, &http.Cookie{
			Name:   cook.Name,
			Value:  cook.Value,
			MaxAge: 60 * 60 * 24,
			Path:   "/",
		})
	}
	ctx.Data(200, " text/html;charset=utf-8", tools.StringToBytes(rs.Content))
}
func (obj *Client) postHandler(ctx *gin.Context) {
	var option RendOption
	err := ctx.BindJSON(&option)
	if err != nil {
		obj.error(ctx, "", 500, err)
		return
	}
	obj.mainHandler(ctx, option)
}
func (obj *Client) getHandler(ctx *gin.Context) {
	var option RendOption
	err := ctx.BindQuery(&option)
	if err != nil {
		obj.error(ctx, "", 500, err)
		return
	}
	obj.mainHandler(ctx, option)
}
func (obj *Client) Run() error {
	for i := 0; i < obj.browserNum; i++ {
		go obj.run()
	}
	cli := gin.Default()
	cli.POST("/render", obj.postHandler)
	cli.GET("/render", obj.getHandler)
	return cli.Run(obj.addr)
}

type RendOption struct {
	Url          string         `json:"url"`
	Proxy        string         `json:"proxy"`
	Sleep        int64          `json:"sleep"`
	WaitSelector string         `json:"waitSelector"`
	Eval         string         `json:"eval"`
	EvalParams   map[string]any `json:"evalParams"`
	cookies      requests.Cookies
}
type taskResult struct {
	Content string       `json:"content"`
	Cookies []cdp.Cookie `json:"cookies"`
}

func (obj *Client) taskMain(_ context.Context, reqCtx context.Context, brow *browser.Client, rendOption RendOption) (taskResult, error) {
	var result taskResult
	var err error
	obj.logCli.Info("new page", map[string]any{"url": rendOption.Url, "proxy": rendOption.Proxy})
	page, err := brow.NewPage(reqCtx, requests.ClientOption{Proxy: rendOption.Proxy})
	if err != nil {
		return result, err
	}
	defer page.Close()
	obj.logCli.Info("new page ok", map[string]any{"url": rendOption.Url, "proxy": rendOption.Proxy})
	if len(rendOption.cookies) > 0 {
		cookies := make([]cdp.Cookie, len(rendOption.cookies))
		for i, cook := range rendOption.cookies {
			cookies[i] = cdp.Cookie{
				Name:   cook.Name,
				Value:  cook.Value,
				Domain: cook.Domain,
				Url:    rendOption.Url,
			}
		}
		obj.logCli.Info("setCookies", map[string]any{"url": rendOption.Url})
		if err = page.SetCookies(reqCtx, cookies...); err != nil {
			return result, err
		}
		obj.logCli.Info("setCookies ok", map[string]any{"url": rendOption.Url})
	}
	obj.logCli.Info("goto", map[string]any{"url": rendOption.Url})
	if err = page.GoTo(reqCtx, rendOption.Url); err != nil {
		return result, err
	}
	obj.logCli.Info("goto ok", map[string]any{"url": rendOption.Url})
	if rendOption.WaitSelector != "" {
		obj.logCli.Info("WaitSelector", map[string]any{"url": rendOption.Url, "WaitSelector": rendOption.WaitSelector})
		if _, err = page.WaitSelector(reqCtx, rendOption.WaitSelector); err != nil {
			return result, err
		}
		obj.logCli.Info("WaitSelector ok", map[string]any{"url": rendOption.Url, "WaitSelector": rendOption.WaitSelector})
	}
	if rendOption.Eval != "" {
		obj.logCli.Info("Eval", map[string]any{"url": rendOption.Url, "Eval": rendOption.Eval})
		if _, err = page.Eval(reqCtx, rendOption.Eval, rendOption.EvalParams); err != nil {
			return result, err
		}
		obj.logCli.Info("Eval ok", map[string]any{"url": rendOption.Url, "Eval": rendOption.Eval})
	}
	if rendOption.Sleep > 0 {
		obj.logCli.Info("sleep", map[string]any{"url": rendOption.Url, "sleep": rendOption.Sleep})
		select {
		case <-reqCtx.Done():
			return result, reqCtx.Err()
		case <-time.After(time.Second * time.Duration(rendOption.Sleep)):
		}
		obj.logCli.Info("sleep ok", map[string]any{"url": rendOption.Url, "sleep": rendOption.Sleep})
	}
	obj.logCli.Info("GetCookies", map[string]any{"url": rendOption.Url})
	result.Cookies, err = page.GetCookies(reqCtx, rendOption.Url)
	if err != nil {
		return result, err
	}
	obj.logCli.Info("GetCookies ok", map[string]any{"url": rendOption.Url})
	obj.logCli.Info("Html", map[string]any{"url": rendOption.Url})
	html, err := page.Html(reqCtx)
	if err != nil {
		return result, err
	}
	obj.logCli.Info("Html ok", map[string]any{"url": rendOption.Url})
	result.Content = html.Html()
	return result, nil
}
func (obj *Client) run() {
	for {
		brow, err := browser.NewClient(obj.ctx, browser.ClientOption{Headless: obj.headless, Proxy: obj.proxy})
		if err != nil {
			log.Print(err)
			time.Sleep(time.Second * 10)
			continue
		}
		obj.logCli.Info("new browser", map[string]any{
			"addr": brow.Addr(),
		})
		pool := thread.NewClient(obj.ctx, obj.pageNum)
	loop:
		for {
			select {
			case <-brow.Done():
				break loop
			case <-obj.ctx.Done():
				return
			case task := <-obj.tasks:
				task.Task, task.err = pool.Write(&thread.Task{
					Func: obj.taskMain,
					Args: []any{task.reqCtx, brow, task.rendOption},
				})
				task.cnl()
				if task.err != nil {
					break loop
				}
			}
		}
		log.Print("浏览器退出")
		pool.Close()
		brow.Close()
		time.Sleep(time.Second * 10)
	}
}
