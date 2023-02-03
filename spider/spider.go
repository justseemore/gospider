package spider

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"gitee.com/baixudong/gospider/blog"
	"gitee.com/baixudong/gospider/browser"
	"gitee.com/baixudong/gospider/cdp"
	"gitee.com/baixudong/gospider/mgo"
	"gitee.com/baixudong/gospider/redis"
	"gitee.com/baixudong/gospider/requests"
	"gitee.com/baixudong/gospider/thread"
	"gitee.com/baixudong/gospider/tools"

	"golang.org/x/exp/slices"
)

type Client struct {
	table   *mgo.Table       // mongodb table
	Session *requests.Client //请求的session
	Browser *browser.Client  //浏览器对象
	Log     string           //日志
	NowTime time.Time        //程序开始运行时间
	ctx     context.Context
	cnl     context.CancelFunc

	GetUrls    func(chan map[string]any)                   //构造列表页数据的函数
	GetLis     func(*Client, map[string]any) ([]Li, error) //构造列表页数据的函数
	GetContent func(*Client, Li) (Content, error)          //详情页自定义返回数据

	LiProcess  int            //协程数量(列表页)
	ConProcess int            //协程数量(详情页)
	SpiderDay  int64          //设置日期截至界限
	TempData   map[string]any //临时数据存放

	update     bool //是否更新已有的数据
	redisCli   *redis.Client
	redisTable string //redis 代理表名
}
type flagData struct {
	mgoHost  *string //mongdb host
	mgoPort  *int    //mongodb port
	mgoUsr   *string //mongodb user
	mgoPwd   *string //mongodb password
	mgoDb    *string //mongodb db name
	mgoTable *string //mongodb table name

	redisHost  *string //redis  host
	redisPort  *int    //redis port
	redisDb    *int    //redis db
	redisPwd   *string //redis password
	redisTable *string //redis table

	proxy  *string //代理url
	day    *int64  //抓最近几天数据
	update *bool   //是否更新历史数据
}
type ClientOption struct {
	Browser       bool                 //是否使用浏览器渲染
	MongoOption   mgo.ClientOption     //mongdb参数
	RedisOption   redis.ClientOption   //redis参数
	BrowserOption browser.ClientOption //浏览器的配置选项

	MgoDb      string //mongdb 数据库名称
	MgoTable   string //mongdb 表名
	RedisTable string //redis 表名

	Proxy string //代理
}
type Li struct {
	Infoid      string         //招标类型
	Title       string         //标题
	OriginUrl   string         //原文链接
	FilterValue string         //去重字段
	PublishTime string         //发布时间 xxxx-xx-xx 格式
	ClearData   map[string]any //k,v 结构化对应关系
	TempData    map[string]any //临时数据存放，不会写入数据库
}
type Content struct {
	Infoid      string         `bson:"infoid"`      //招标类型
	Title       string         `bson:"title"`       //标题
	OriginUrl   string         `bson:"originUrl"`   //原文链接
	FilterValue string         `bson:"filterValue"` //去重字段
	Content     string         `bson:"content"`     //内容html
	PublishTime string         `bson:"publishTime"` //发布时间 xxxx-xx-xx 格式
	SpiderTime  int64          `bson:"spiderTime"`  //爬虫运行时间，自动设置不需要赋值
	AreaStr     string         `bson:"areaStr"`     //地区
	ClearData   map[string]any `bson:"clearData"`   //k,v 结构化对应关系
	SpiderId    int            `bson:"spiderId"`    //爬虫id
}

func (obj *Client) getProxy() (string, error) {
	proxy, err := obj.redisCli.GetProxy(obj.redisTable)
	return "http://" + proxy, err
}
func NewClient(preCtx context.Context, options ...ClientOption) (*Client, error) {
	var option ClientOption
	if len(options) > 0 {
		option = options[0]
	}
	if preCtx == nil {
		preCtx = context.TODO()
	}
	ctx, cnl := context.WithCancel(preCtx)
	client := Client{
		ctx: ctx,
		cnl: cnl,
	}
	var err error
	client.loadConfig(&option)
	client.redisCli, err = redis.NewClient(option.RedisOption)
	if err != nil {
		return nil, err
	}
	mgoCli, err := mgo.NewClient(ctx, option.MongoOption)
	if err != nil {
		return nil, err
	}
	client.table = mgoCli.NewTable(option.MgoDb, option.MgoTable)
	if option.Browser {
		client.Browser, err = browser.NewClient(ctx, option.BrowserOption)
		if err != nil {
			client.addLog("create browser error: " + err.Error())
			return nil, err
		}
	}
	//当前时间
	client.NowTime = time.Now()
	//创建请求对象
	if client.Session, err = requests.NewClient(ctx, requests.ClientOption{
		Proxy:    option.Proxy,
		GetProxy: func(ctx context.Context, url *url.URL) (string, error) { return client.getProxy() },
	}); err != nil {
		return nil, err
	}
	return &client, err
}
func (obj *Client) loadConfig(option *ClientOption) { //加载配置
	fla := obj.flagConfig()
	//mongodb 参数配置读取
	if len(os.Args)-1 != len(flag.Args()) {
		option.MongoOption.Host = *fla.mgoHost
		option.MongoOption.Port = *fla.mgoPort
		option.MongoOption.Usr = *fla.mgoUsr
		option.MongoOption.Pwd = *fla.mgoPwd
		option.MgoDb = *fla.mgoDb
		option.MgoTable = *fla.mgoTable
		option.RedisOption.Host = *fla.redisHost
		option.RedisOption.Port = *fla.redisPort
		option.RedisOption.Db = *fla.redisDb
		option.RedisOption.Pwd = *fla.redisPwd
		option.RedisTable = *fla.redisTable
		option.Proxy = *fla.proxy
		obj.SpiderDay = *fla.day
		obj.update = *fla.update
	}
	if obj.SpiderDay == 0 {
		obj.SpiderDay = 3
	}
	if option.RedisTable == "" {
		obj.redisTable = "proxies_hash"
	} else {
		obj.redisTable = option.RedisTable
	}

	if option.MgoDb == "" {
		option.MgoDb = "spider"
	}
	if option.MgoTable == "" {
		option.MgoTable = "zhaoBiaoData"
	}
}
func (obj *Client) flagConfig() flagData { //加载命令行配置
	result := flagData{
		mgoHost:  flag.String("mgoHost", "", "mongodb host"),
		mgoPort:  flag.Int("mgoPort", 0, "mongodb port"),
		mgoUsr:   flag.String("mgoUsr", "", "mongodb user"),
		mgoPwd:   flag.String("mgoPwd", "", "mongodb password"),
		mgoDb:    flag.String("mgoDb", "", "mongodb的db名字"),
		mgoTable: flag.String("mgoTable", "", "mongodb的table名字"),

		redisHost: flag.String("redisHost", "", "redis host"),
		redisPort: flag.Int("redisPort", 0, "redis port"),

		redisPwd:   flag.String("redisPwd", "", "redis password"),
		redisDb:    flag.Int("redisDb", 0, "redis的db名字"),
		redisTable: flag.String("redisTable", "", "redis的table名字"),

		proxy:  flag.String("proxy", "", "指定代理"),
		day:    flag.Int64("day", 0, "爬取天数,默认抓取3天"),
		update: flag.Bool("update", false, "是否更新历史数据"),
	}
	flag.Parse()
	return result
}
func (obj *Client) addLog(val string) error {
	log.Print(blog.Color(1, 0, val))
	obj.Log += val + "\n"
	return errors.New(val)
}
func (obj *Client) verifyConfig() error {
	if obj.table == nil {
		obj.addLog("Table 缺失，请连接mongodb")
		return errors.New("not found mongodb table")
	}
	if obj.GetUrls == nil {
		obj.addLog("not found GetUrls")
		return errors.New("not found GetUrls")
	}
	if obj.GetLis == nil {
		obj.addLog("not found GetLis")
		return errors.New("not found GetLis")
	}
	if obj.GetContent == nil {
		obj.addLog("not found GetContent")
		return errors.New("not found GetContent")
	}
	return nil
}

// 发送请求
func (obj *Client) Request(ctx context.Context, method string, url string, option requests.RequestOption, isBrowser ...bool) (*requests.Response, error) {
	if len(isBrowser) > 0 && !isBrowser[0] {
		return obj.Session.Request(ctx, method, url, option)
	}
	result := new(requests.Response)
	if obj.Browser == nil {
		return nil, errors.New("浏览器没有初始化")
	}
	page, err := obj.Browser.NewPage(ctx)
	if err != nil {
		return nil, err
	}
	defer page.Close()
	page.ReqCli = obj.Session
	page.Route(ctx, func(ctx context.Context, r *cdp.Route) {
		rs, err := r.Request(ctx, r.NewRequestOption(), option)
		if err != nil {
			r.Fail(ctx, "Failed")
		} else {
			r.FulFill(ctx, rs)
		}
	})
	if err = page.GoTo(ctx, url); err != nil {
		return nil, err
	}
	content, err := page.Html(ctx)
	if err != nil {
		return nil, err
	}
	cookies, err := page.GetCookies(ctx)
	if err != nil {
		return nil, err
	}
	for _, cookie := range cookies {
		obj.Session.Cookies(url, &http.Cookie{
			Name:     cookie.Name,
			Value:    cookie.Value,
			Path:     cookie.Path,
			Domain:   cookie.Domain,
			Secure:   cookie.Secure,
			HttpOnly: cookie.HttpOnly,
		})
	}
	result.Content([]byte(tools.StringToBytes(content.Html())))
	return result, nil
}

// 开始运行
func (obj *Client) Run() error {
	if err := obj.verifyConfig(); err != nil {
		return err
	}
	urlDatas := make(chan map[string]any)
	go obj.getUrls(urlDatas)
	var pool *thread.DefaultClient
	if obj.LiProcess > 1 {
		pool = thread.NewClient(obj.ctx, obj.LiProcess)
		defer pool.Join()
	}
	for {
		select {
		case <-obj.ctx.Done():
			return obj.ctx.Err()
		case urlData := <-urlDatas:
			if urlData == nil {
				return nil
			}
			if pool == nil {
				if err := obj.getLis(obj.ctx, urlData); err != nil {
					return err
				}
			} else {
				_, err := pool.Write(&thread.Task{
					Func: obj.getLis,
					Args: []any{urlData},
				})
				if err != nil {
					return err
				}
			}
		}
	}
}
func (obj *Client) getUrls(urlData chan map[string]any) {
	defer close(urlData)
	obj.GetUrls(urlData)
}
func (obj *Client) getLis(con context.Context, urlData map[string]any) error { //协程开始
	log.Printf("列表请求开始:urlData:=%#v", urlData)
	lis, err := obj.GetLis(obj, urlData)
	if err != nil {
		obj.Close()
		return obj.addLog(fmt.Sprintf("列表请求失败:urlData:=%#v", urlData))
	}
	if len(lis) == 0 {
		obj.Close()
		return obj.addLog(fmt.Sprintf("列表元素提取错误:urlData:=%#v", urlData))
	}
	log.Print(blog.Color(2, 0, fmt.Sprintf("列表请求成功:urlData:=%#v", urlData)))
	var pool *thread.DefaultClient
	if obj.ConProcess > 0 {
		pool = thread.NewClient(con, obj.ConProcess)
	} else {
		pool = thread.NewClient(con, len(lis))
	}
	results := []string{}
	tasks := []*thread.Task{}
	task_txts := []string{"已采集", "日期超过界限", "符合采集要求", "不同批次重复"}
	for _, li := range lis {
		if li.FilterValue == "" {
			li.FilterValue = li.OriginUrl
		}
		filter_rs, isGet := obj.filter(&li)
		if isGet {
			tempTask, err := pool.Write(&thread.Task{
				Func: obj.getContent,
				Args: []any{li},
			})
			if err != nil {
				break
			}
			tasks = append(tasks, tempTask)
		} else {
			_, inOk := slices.BinarySearch(task_txts, filter_rs)
			if !inOk {
				obj.addLog(fmt.Sprintf("列表页不正常:err:=%s\nurlData:=%#v\nli_data:=%s", filter_rs, urlData, tools.Any2json(li)))
			}
			results = append(results, filter_rs)
		}
	}
	if pool != nil {
		pool.Join()
	}
	for _, task := range tasks {
		result := task.Result
		if len(result) == 0 {
			err_txt := task.Error.Error()
			obj.addLog(fmt.Sprintf("提取内容异常:err:=%s\nurlData:=%#v\nli_data:=%s", err_txt, urlData, tools.Any2json(task.Args[0].(Li))))
			results = append(results, err_txt)
		} else {
			task_txt := task.Result[0].(string)
			_, inOk := slices.BinarySearch(task_txts, task_txt)
			if !inOk {
				obj.addLog(fmt.Sprintf("提取内容异常:err:=%s\nurlData:=%#v\nli_data:=%s", task_txt, urlData, tools.Any2json(task.Args[0].(Li))))
			}
			results = append(results, task_txt)
		}
	}
	log.Print(blog.Color(3, 0, results))
	if !obj.panNext(results) {
		obj.Close()
		return errors.New("next break")
	}
	return nil
}
func (obj *Client) getContent(con context.Context, li Li) string {
	log.Printf("详情请求开始start:OriginUrl:=%s", li.OriginUrl)
	content, err := obj.GetContent(obj, li)
	log.Print(blog.Color(2, 0, fmt.Sprintf("详情请求结束end:OriginUrl:=%s", li.OriginUrl)))
	if err != nil {
		return err.Error()
	}
	if content.FilterValue == "" {
		content.FilterValue = li.FilterValue
	}
	if content.ClearData == nil {
		content.ClearData = li.ClearData
	} else if li.ClearData != nil {
		for k, v := range content.ClearData {
			li.ClearData[k] = v
		}
		content.ClearData = li.ClearData
	}
	if content.Title == "" {
		content.Title = li.Title
	}
	if content.PublishTime == "" {
		content.PublishTime = li.PublishTime
	}
	if content.OriginUrl == "" {
		content.OriginUrl = li.OriginUrl
	}
	if err != nil {
		return "内容请求失败"
	}
	if content.Content == "" {
		return "没有提取到内容"
	}
	if content.Title == "" {
		return "没有提取到标题"
	}
	if content.PublishTime == "" {
		return "没有提取到时间"
	}
	if content.OriginUrl == "" {
		return "没有提取到链接"
	}
	obj.insert(content)
	return obj.dateBj(content.PublishTime)
}
func (obj *Client) insert(data Content) {
	data.SpiderTime = obj.NowTime.Unix()
	obj.table.Upsert(obj.ctx, map[string]string{
		"filterValue": data.FilterValue,
	}, data)
}
func (obj *Client) filter(data *Li) (string, bool) {
	if data.OriginUrl == "" {
		return "内容链接不存在", false
	}
	fd, _ := obj.table.Find(obj.ctx, map[string]string{"filterValue": data.FilterValue}, mgo.FindOption{
		Show: map[string]int{
			"_id":         1,
			"publishTime": 1,
			"spiderTime":  1,
		}},
	)
	if fd != nil { //有数据
		publishTime, ok := fd.Data()["publishTime"]
		if ok {
			data.PublishTime = publishTime.(string)
		}
		spiderTime, ok := fd.Data()["spiderTime"]
		if ok && obj.NowTime.Unix() == spiderTime.(int64) {
			return "同一批次重复", false
		}
	}
	if data.PublishTime != "" {
		bj_rs := obj.dateBj(data.PublishTime)
		if obj.update && !strings.Contains(bj_rs, "错误") {
			return "符合采集要求", true
		}
		if bj_rs == "符合采集要求" {
			if fd == nil {
				return "符合采集要求", true
			} else {
				return "不同批次重复", false
			}
		} else { //日期问题错误，不采集
			return bj_rs, false
		}
	} else {
		return "符合采集要求", true
	}
}
func (obj *Client) panNext(data []string) bool {
	for _, txt := range data {
		if txt == "符合采集要求" || txt == "不同批次重复" {
			return true
		}
	}
	return false
}
func (obj *Client) dateBj(publishTime string) string {
	publishTime_format, err := time.ParseInLocation(time.DateOnly, publishTime, time.Local)
	if err != nil {
		return "日期格式错误"
	} else {
		sub_day := obj.NowTime.Unix()/(60*60*24) - publishTime_format.Unix()/(60*60*24)
		if sub_day < 0 {
			return "日期格式错误"
		} else if obj.SpiderDay == -1 {
			return "符合采集要求"
		} else {
			if sub_day >= obj.SpiderDay {
				return "日期超过界限"
			} else {
				return "符合采集要求"
			}
		}
	}
}

// 关闭客户端
func (obj *Client) Close() {
	if obj.cnl != nil {
		obj.cnl()
	}
	if obj.Session != nil {
		obj.Session.Close()
	}
	if obj.Browser != nil {
		obj.Browser.Close()
	}
}
