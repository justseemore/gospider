package requests

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"gitee.com/baixudong/gospider/tools"
	"gitee.com/baixudong/gospider/websocket"

	"github.com/tidwall/gjson"
)

var UserAgent = `Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/109.0.0.0 Safari/537.36`

// 请求操作========================================================================= start
var defaultHeaders = http.Header{
	"Accept-encoding": []string{"gzip, deflate, br"},
	"Accept":          []string{"text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8"},
	"Accept-Language": []string{"zh-CN,zh;q=0.9"},
	"User-Agent":      []string{UserAgent},
}

type myInt int

const (
	keyPrincipalID myInt = iota
)

var (
	errFatal = errors.New("致命错误")
)

type reqCtxData struct {
	proxyUser   *url.Userinfo
	proxy       *url.URL
	url         *url.URL
	redirectNum int
	disProxy    bool
	h2          bool
	ja3         bool
}
type File struct {
	Key     string //字段的key
	Name    string //文件名
	Content []byte //文件的内容
	Type    string //文件类型
}

type RequestOption struct {
	Method        string //method
	Url           string //url
	Host          string //host
	Proxy         string //代理,http,socks5
	Timeout       int64  //请求超时时间
	Headers       any    //请求头
	Cookies       any    // cookies
	Files         []File //文件
	Params        any    //url params,url 参数key,val
	Form          any    //multipart/form-data,适用于文件上传
	Data          any    //application/x-www-form-urlencoded,适用于key,val
	Body          *bytes.Reader
	Json          any                                       //application/json
	Text          any                                       //text/xml
	TempData      any                                       //临时变量
	Bytes         []byte                                    //二进制内容
	DisCookie     bool                                      //关闭cookies管理
	DisDecode     bool                                      //关闭自动解码
	Bar           bool                                      //是否开启bar
	DisProxy      bool                                      //是否关闭代理
	Ja3           bool                                      //是否开启ja3
	TryNum        int64                                     //重试次数
	CurTryNum     int64                                     //当前尝试次数
	BeforCallBack func(*RequestOption)                      //请求之前回调
	AfterCallBack func(*RequestOption, *Response) *Response //请求之后回调
	RedirectNum   int                                       //重定向次数
	DisAlive      bool                                      //关闭长连接
	DisRead       bool                                      //关闭默认读取请求体
	DisUnZip      bool                                      //变比自动解压
	Err           error                                     //请求过程中的error
	Http2         bool                                      //开启http2 transport
	WsOption      websocket.ClientOption                    //websocket option
	converUrl     string
	contentType   string
}

func newBody(val any, valType string, dataMap map[string][]string) (*bytes.Reader, error) {
	switch value := val.(type) {
	case gjson.Result:
		if !value.IsObject() {
			return nil, errors.New("body-type错误")
		}
		switch valType {
		case "json", "text":
			return bytes.NewReader(tools.StringToBytes(value.Raw)), nil
		case "data", "params":
			tempVal := url.Values{}
			for kk, vv := range value.Map() {
				if vv.IsArray() {
					for _, v := range vv.Array() {
						tempVal.Add(kk, v.String())
					}
				} else {
					tempVal.Add(kk, vv.String())
				}
			}
			return bytes.NewReader(tools.StringToBytes(tempVal.Encode())), nil
		case "form":
			for kk, vv := range value.Map() {
				kkvv := []string{}
				if vv.IsArray() {
					for _, v := range vv.Array() {
						kkvv = append(kkvv, v.String())
					}
				} else {
					kkvv = append(kkvv, vv.String())
				}
				dataMap[kk] = kkvv
			}
			return nil, nil
		default:
			return nil, errors.New("未知的content-type：" + valType)
		}
	case string:
		switch valType {
		case "json", "text", "data":
			return bytes.NewReader(tools.StringToBytes(value)), nil
		default:
			return nil, errors.New("未知的content-type：" + valType)
		}
	case []byte:
		switch valType {
		case "json", "text", "data":
			return bytes.NewReader(value), nil
		default:
			return nil, errors.New("未知的content-type：" + valType)
		}
	case io.Reader:
		tempCon, err := io.ReadAll(value)
		return bytes.NewReader(tempCon), err
	default:
		return newBody(tools.Any2json(value), valType, dataMap)
	}
}
func (obj *RequestOption) newHeaders() error {
	if obj.Headers == nil {
		obj.Headers = defaultHeaders.Clone()
		return nil
	}
	switch headers := obj.Headers.(type) {
	case http.Header:
		return nil
	case gjson.Result:
		if !headers.IsObject() {
			return errors.New("new headers error")
		}
		head := http.Header{}
		for kk, vv := range headers.Map() {
			if vv.IsArray() {
				for _, v := range vv.Array() {
					head.Add(kk, v.String())
				}
			} else {
				head.Add(kk, vv.String())
			}
		}
		obj.Headers = head
		return nil
	default:
		obj.Headers = tools.Any2json(headers)
		return obj.newHeaders()
	}
}
func (obj *RequestOption) newCookies() error {
	if obj.Cookies == nil {
		return nil
	}
	switch cookies := obj.Cookies.(type) {
	case []*http.Cookie:
		return nil
	case gjson.Result:
		if !cookies.IsObject() {
			return errors.New("new cookies error")
		}
		cook := []*http.Cookie{}
		for kk, vv := range cookies.Map() {
			if vv.IsArray() {
				for _, v := range vv.Array() {
					cook = append(cook, &http.Cookie{
						Name:  kk,
						Value: v.String(),
					})
				}
			} else {
				cook = append(cook, &http.Cookie{
					Name:  kk,
					Value: vv.String(),
				})
			}
		}
		obj.Cookies = cook
		return nil
	default:
		obj.Cookies = tools.Any2json(cookies)
		return obj.newCookies()
	}
}
func (obj *RequestOption) optionInit() error {
	obj.converUrl = obj.Url
	var err error
	if obj.Bytes != nil {
		obj.Body = bytes.NewReader(obj.Bytes)
	}
	if obj.Form != nil {
		tempBody := bytes.NewBuffer(nil)
		dataMap := map[string][]string{}
		obj.Body, err = newBody(obj.Form, "form", dataMap)
		if err != nil {
			return err
		}
		writer := multipart.NewWriter(tempBody)
		for key, vals := range dataMap {
			for _, val := range vals {
				err := writer.WriteField(key, val)
				if err != nil {
					return err
				}
			}
		}
		escapeQuotes := strings.NewReplacer("\\", "\\\\", `"`, "\\\"")
		for _, file := range obj.Files {
			h := make(textproto.MIMEHeader)
			h.Set("Content-Disposition",
				fmt.Sprintf(`form-data; name="%s"; filename="%s"`,
					escapeQuotes.Replace(file.Key), escapeQuotes.Replace(file.Name)))
			if file.Type == "" {
				h.Set("Content-Type", "application/octet-stream")
			} else {
				h.Set("Content-Type", file.Type)
			}
			wp, err := writer.CreatePart(h)
			if err != nil {
				return err
			}
			_, err = wp.Write(file.Content)
			if err != nil {
				return err
			}
		}
		err = writer.Close()
		if err != nil {
			return err
		}
		obj.contentType = writer.FormDataContentType()
		temCon, err := io.ReadAll(tempBody)
		if err != nil {
			return err
		}
		obj.Body = bytes.NewReader(temCon)
	}
	if obj.Data != nil {
		obj.Body, err = newBody(obj.Data, "data", nil)
		if err != nil {
			return err
		}
		obj.contentType = "application/x-www-form-urlencoded"
	}
	if obj.Json != nil {
		obj.Body, err = newBody(obj.Json, "json", nil)
		if err != nil {
			return err
		}
		obj.contentType = "application/json"
	}
	if obj.Text != nil {
		obj.Body, err = newBody(obj.Text, "text", nil)
		if err != nil {
			return err
		}
		obj.contentType = "text/plain"
	}
	if obj.Params != nil {

		tempParam, err := newBody(obj.Params, "params", nil)
		if err != nil {
			return err
		}
		con, err := io.ReadAll(tempParam)
		if err != nil {
			return err
		}
		pu, err := url.Parse(obj.Url)
		if err != nil {
			return err
		}
		if pu.Query() == nil {
			obj.converUrl = obj.Url + "?" + tools.BytesToString(con)
		} else {
			obj.converUrl = obj.Url + "&" + tools.BytesToString(con)
		}
	}
	if err = obj.newHeaders(); err != nil {
		return err
	}
	return obj.newCookies()
}

func (obj *Client) newRequestOption(option *RequestOption) {
	if option.TryNum == 0 {
		option.TryNum = obj.TryNum
	}
	if option.BeforCallBack == nil {
		option.BeforCallBack = obj.BeforCallBack
	}
	if option.AfterCallBack == nil {
		option.AfterCallBack = obj.AfterCallBack
	}
	if option.Headers == nil {
		if obj.Headers == nil {
			option.Headers = defaultHeaders.Clone()
		} else {
			option.Headers = obj.Headers
		}
	}
	if !option.Bar {
		option.Bar = obj.Bar
	}
	if option.RedirectNum == 0 {
		option.RedirectNum = obj.RedirectNum
	}
	if option.Timeout == 0 {
		option.Timeout = obj.Timeout
	}
	if !option.DisAlive {
		option.DisAlive = obj.disAlive
	}
	if !option.DisCookie {
		option.DisCookie = obj.disCookie
	}
	if !option.DisDecode {
		option.DisDecode = obj.DisDecode
	}
	if !option.DisRead {
		option.DisRead = obj.DisRead
	}
	if !option.DisUnZip {
		option.DisUnZip = obj.DisUnZip
	}
	if !option.Http2 {
		option.Http2 = obj.Http2
	}
	if !option.Ja3 {
		option.Ja3 = obj.Ja3
	}
}

func (obj *Client) Request(preCtx context.Context, method string, href string, options ...RequestOption) (*Response, error) {
	if obj == nil {
		return nil, errors.New("初始化client失败")
	}
	if preCtx == nil {
		preCtx = obj.ctx
	}
	var option RequestOption
	if len(options) > 0 {
		option = options[0]
	}
	var err error
	if option.Method == "" {
		option.Method = method
	}
	if option.Url == "" {
		option.Url = href
	}
	if option.Body != nil {
		option.TryNum = 0
	}
	obj.newRequestOption(&option)
	if option.BeforCallBack == nil {
		if err = option.optionInit(); err != nil {
			return nil, err
		}
	}
	//开始请求
	var resp *Response
	for ; option.CurTryNum <= option.TryNum; option.CurTryNum++ {
		select {
		case <-preCtx.Done():
			return nil, preCtx.Err()
		default:
			if option.BeforCallBack != nil {
				option.BeforCallBack(&option)
			}
			if option.BeforCallBack != nil || option.AfterCallBack != nil {
				obj.newRequestOption(&option)
				if err = option.optionInit(); err != nil {
					return nil, err
				}
			}
			if option.Body != nil {
				option.Body.Seek(0, 0)
			}
			resp, option.Err = obj.tempRequest(preCtx, option)
			if option.Err != nil && errors.Is(option.Err, errFatal) {
				return resp, option.Err
			}
			if option.AfterCallBack != nil {
				callBackRespon := option.AfterCallBack(&option, resp)
				if callBackRespon != nil && option.Err == nil {
					return callBackRespon, option.Err
				}
			} else if option.Err == nil {
				return resp, option.Err
			}
		}
	}
	if option.Err != nil {
		return resp, option.Err
	}
	return resp, errors.New("max try num")
}
func verifyProxy(proxyUrl string) (*url.URL, error) {
	proxy, err := url.Parse(proxyUrl)
	if err != nil {
		return nil, err
	}
	switch proxy.Scheme {
	case "http", "socks5":
		return proxy, nil
	default:
		return nil, tools.WrapError(errFatal, "不支持的代理协议")
	}
}

func (obj *Client) tempRequest(preCtx context.Context, request_option RequestOption) (response *Response, err error) {
	method := strings.ToUpper(request_option.Method)
	href := request_option.converUrl
	var reqs *http.Request
	var isWs bool
	//构造ctxData
	ctxData := new(reqCtxData)
	ctxData.disProxy = request_option.DisProxy
	ctxData.h2 = request_option.Http2
	ctxData.ja3 = request_option.Ja3
	if request_option.Proxy != "" { //代理相关构造
		tempProxy, err := verifyProxy(request_option.Proxy)
		if err != nil {
			return response, tools.WrapError(errFatal, err)
		}
		ctxData.proxy = tempProxy
	}
	if request_option.RedirectNum != 0 { //重定向次数
		ctxData.redirectNum = request_option.RedirectNum
	}
	//构造ctx,cnl
	var cancel context.CancelFunc
	reqCtx := context.WithValue(preCtx, keyPrincipalID, ctxData)
	if request_option.Timeout > 0 { //超时
		reqCtx, cancel = context.WithTimeout(reqCtx, time.Duration(request_option.Timeout)*time.Second)
	} else {
		reqCtx, cancel = context.WithCancel(reqCtx)
	}
	defer func() {
		if err != nil {
			cancel()
			if response != nil {
				response.Close()
			}
		}
	}()
	//创建request
	if request_option.Body != nil {
		reqs, err = http.NewRequestWithContext(reqCtx, method, href, request_option.Body)
	} else {
		reqs, err = http.NewRequestWithContext(reqCtx, method, href, nil)
	}
	if err != nil {
		return response, tools.WrapError(errFatal, err)
	}
	ctxData.url = reqs.URL
	//判断ws
	switch reqs.URL.Scheme {
	case "ws":
		isWs = true
		reqs.URL.Scheme = "http"
	case "wss":
		isWs = true
		reqs.URL.Scheme = "https"
	}
	//添加headers
	var headOk bool
	if reqs.Header, headOk = request_option.Headers.(http.Header); !headOk {
		return response, tools.WrapError(errFatal, "headers 转换错误")
	}

	if !isWs && reqs.Header.Get("Content-type") == "" && request_option.contentType != "" {
		reqs.Header.Add("Content-Type", request_option.contentType)
	}
	//host构造
	if request_option.Host != "" {
		reqs.Host = request_option.Host
	} else if reqs.Header.Get("Host") != "" {
		reqs.Host = reqs.Header.Get("Host")
	}

	//添加cookies
	if request_option.Cookies != nil {
		cooks, cookOk := request_option.Cookies.([]*http.Cookie)
		if !cookOk {
			return response, tools.WrapError(errFatal, "cookies 转换错误")
		}
		for _, vv := range cooks {
			reqs.AddCookie(vv)
		}
	}
	if !request_option.Http2 {
		reqs.Close = request_option.DisAlive
	}
	//开始发送请求
	var r *http.Response
	var err2 error
	if isWs {
		if err = websocket.SetClientHeaders(reqs.Header, &request_option.WsOption); err != nil {
			return response, tools.WrapError(errFatal, err.Error())
		}
	}
	r, err = obj.getClient(request_option).Do(reqs)
	if r != nil {
		if isWs {
			if r.StatusCode == 101 {
				request_option.DisRead = true
			} else if err == nil {
				err = errors.New("statusCode not 101")
			}
		}
		r.Close = request_option.DisAlive
		if response, err2 = obj.newResponse(r, cancel, request_option); err2 != nil { //创建 response
			return response, err2
		}
		if isWs && r.StatusCode == 101 {
			if response.webSocket, err2 = websocket.NewClientConn(r, &request_option.WsOption); err2 != nil { //创建 websocket
				return response, err2
			}
		}
	}
	return response, err
}
