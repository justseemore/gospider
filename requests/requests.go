package requests

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"net/http"
	"net/url"
	"os"
	"strings"
	_ "unsafe"

	"github.com/justseemore/gospider/re"
	"github.com/justseemore/gospider/tools"
	"github.com/justseemore/gospider/websocket"
)

//go:linkname ReadRequest net/http.readRequest
func ReadRequest(b *bufio.Reader) (*http.Request, error)

type RequestDebug struct {
	Url    *url.URL
	Method string
	Header http.Header
	Proto  string
	con    *bytes.Buffer
}

func (obj *RequestDebug) request() (*http.Request, error) {
	return ReadRequest(bufio.NewReader(bytes.NewBuffer(obj.con.Bytes())))
}
func (obj *RequestDebug) String() string {
	return obj.con.String()
}
func (obj *RequestDebug) Bytes() []byte {
	return obj.con.Bytes()
}
func (obj *RequestDebug) body() (io.ReadCloser, error) {
	r, err := obj.request()
	if err != nil {
		return nil, err
	}
	return r.Body, err
}
func (obj *RequestDebug) Body() (*bytes.Buffer, error) {
	body, err := obj.body()
	if err != nil {
		return nil, err
	}
	con := bytes.NewBuffer(nil)
	_, err = io.Copy(con, body)
	return con, err
}
func cloneRequest(r *http.Request, disBody bool) (*RequestDebug, error) {
	request := new(RequestDebug)
	request.Proto = r.Proto
	request.Method = r.Method
	request.Url = r.URL
	request.Header = r.Header
	var err error
	request.con = bytes.NewBuffer(nil)
	if !disBody {
		if err = r.Write(request.con); err != nil {
			return request, err
		}
		r.Body, err = request.body()
	} else {
		if _, err = request.con.WriteString(fmt.Sprintf("%s %s %s\r\n", r.Method, r.URL, r.Proto)); err != nil {
			return request, err
		}
		if err = r.Header.Write(request.con); err != nil {
			return request, err
		}
		if _, err = request.con.WriteString("\r\n"); err != nil {
			return request, err
		}
	}
	return request, err
}

type ResponseDebug struct {
	Proto   string
	Status  string
	Header  http.Header
	con     *bytes.Buffer
	request *http.Request
}

func (obj *ResponseDebug) response() (*http.Response, error) {
	return http.ReadResponse(bufio.NewReader(bytes.NewBuffer(obj.con.Bytes())), obj.request)
}
func (obj *ResponseDebug) String() string {
	return obj.con.String()
}
func (obj *ResponseDebug) Bytes() []byte {
	return obj.con.Bytes()
}
func (obj *ResponseDebug) body() (io.ReadCloser, error) {
	r, err := obj.response()
	if err != nil {
		return nil, err
	}
	return r.Body, err
}
func (obj *ResponseDebug) Body() (*bytes.Buffer, error) {
	body, err := obj.body()
	if err != nil {
		return nil, err
	}
	con := bytes.NewBuffer(nil)
	_, err = io.Copy(con, body)
	return con, err
}
func cloneResponse(r *http.Response, disBody bool) (*ResponseDebug, error) {
	response := new(ResponseDebug)
	response.con = bytes.NewBuffer(nil)
	response.Proto = r.Proto
	response.Status = r.Status
	response.Header = r.Header
	response.request = r.Request

	var err error
	response.con = bytes.NewBuffer(nil)
	if !disBody {
		if err = r.Write(response.con); err != nil {
			return response, err
		}
		r.Body, err = response.body()
	} else {
		if _, err = response.con.WriteString(fmt.Sprintf("%s %s\r\n", r.Proto, r.Status)); err != nil {
			return response, err
		}
		if err = r.Header.Write(response.con); err != nil {
			return response, err
		}
		if _, err = response.con.WriteString("\r\n"); err != nil {
			return response, err
		}
	}
	return response, err
}

const keyPrincipalID = "gospiderContextData"

var (
	ErrFatal = errors.New("致命错误")
)

type reqCtxData struct {
	isCallback       bool
	proxy            *url.URL
	url              *url.URL
	host             string
	redirectNum      int
	disProxy         bool
	ws               bool
	requestCallBack  func(context.Context, *RequestDebug) error
	disBody          bool
	responseCallBack func(context.Context, *ResponseDebug) error
}

func Get(preCtx context.Context, href string, options ...RequestOption) (*Response, error) {
	client, _ := NewClient(preCtx)
	defer client.Close()
	return client.Request(preCtx, http.MethodGet, href, options...)
}
func Head(preCtx context.Context, href string, options ...RequestOption) (*Response, error) {
	client, _ := NewClient(preCtx)
	defer client.Close()
	return client.Request(preCtx, http.MethodHead, href, options...)
}
func Post(preCtx context.Context, href string, options ...RequestOption) (*Response, error) {
	client, _ := NewClient(preCtx)
	defer client.Close()
	return client.Request(preCtx, http.MethodPost, href, options...)
}
func Put(preCtx context.Context, href string, options ...RequestOption) (*Response, error) {
	client, _ := NewClient(preCtx)
	defer client.Close()
	return client.Request(preCtx, http.MethodPut, href, options...)
}
func Patch(preCtx context.Context, href string, options ...RequestOption) (*Response, error) {
	client, _ := NewClient(preCtx)
	defer client.Close()
	return client.Request(preCtx, http.MethodPatch, href, options...)
}
func Delete(preCtx context.Context, href string, options ...RequestOption) (*Response, error) {
	client, _ := NewClient(preCtx)
	defer client.Close()
	return client.Request(preCtx, http.MethodDelete, href, options...)
}
func Connect(preCtx context.Context, href string, options ...RequestOption) (*Response, error) {
	client, _ := NewClient(preCtx)
	defer client.Close()
	return client.Request(preCtx, http.MethodConnect, href, options...)
}
func Options(preCtx context.Context, href string, options ...RequestOption) (*Response, error) {
	client, _ := NewClient(preCtx)
	defer client.Close()
	return client.Request(preCtx, http.MethodOptions, href, options...)
}
func Trace(preCtx context.Context, href string, options ...RequestOption) (*Response, error) {
	client, _ := NewClient(preCtx)
	defer client.Close()
	return client.Request(preCtx, http.MethodTrace, href, options...)
}
func Request(preCtx context.Context, method string, href string, options ...RequestOption) (*Response, error) {
	client, _ := NewClient(preCtx)
	defer client.Close()
	return client.Request(preCtx, method, href, options...)
}
func (obj *Client) Get(preCtx context.Context, href string, options ...RequestOption) (*Response, error) {
	return obj.Request(preCtx, http.MethodGet, href, options...)
}
func (obj *Client) Head(preCtx context.Context, href string, options ...RequestOption) (*Response, error) {
	return obj.Request(preCtx, http.MethodHead, href, options...)
}
func (obj *Client) Post(preCtx context.Context, href string, options ...RequestOption) (*Response, error) {
	return obj.Request(preCtx, http.MethodPost, href, options...)
}
func (obj *Client) Put(preCtx context.Context, href string, options ...RequestOption) (*Response, error) {
	return obj.Request(preCtx, http.MethodPut, href, options...)
}
func (obj *Client) Patch(preCtx context.Context, href string, options ...RequestOption) (*Response, error) {
	return obj.Request(preCtx, http.MethodPatch, href, options...)
}
func (obj *Client) Delete(preCtx context.Context, href string, options ...RequestOption) (*Response, error) {
	return obj.Request(preCtx, http.MethodDelete, href, options...)
}
func (obj *Client) Connect(preCtx context.Context, href string, options ...RequestOption) (*Response, error) {
	return obj.Request(preCtx, http.MethodConnect, href, options...)
}
func (obj *Client) Options(preCtx context.Context, href string, options ...RequestOption) (*Response, error) {
	return obj.Request(preCtx, http.MethodOptions, href, options...)
}
func (obj *Client) Trace(preCtx context.Context, href string, options ...RequestOption) (*Response, error) {
	return obj.Request(preCtx, http.MethodTrace, href, options...)
}

// 发送请求
func (obj *Client) Request(preCtx context.Context, method string, href string, options ...RequestOption) (resp *Response, err error) {
	if obj == nil {
		return nil, errors.New("client is nil")
	}
	if preCtx == nil {
		preCtx = obj.ctx
	}
	var rawOption RequestOption
	if len(options) > 0 {
		rawOption = options[0]
	}
	optionBak := obj.newRequestOption(rawOption)
	if rawOption.Body != nil && optionBak.TryNum > 0 {
		optionBak.TryNum = 0
	}
	//开始请求
	var tryNum int64
	for tryNum = 0; tryNum <= optionBak.TryNum; tryNum++ {
		select {
		case <-obj.ctx.Done():
			obj.Close()

			return nil, tools.WrapError(obj.ctx.Err(), "client ctx 错误")
		case <-preCtx.Done():
			return nil, tools.WrapError(preCtx.Err(), "request ctx 错误")
		default:
			option := optionBak
			if option.Method == "" {
				option.Method = method
			}
			if option.Url == nil {
				if option.Url, err = url.Parse(href); err != nil {
					err = tools.WrapError(err, "url 解析错误")
					return
				}
			}
			if option.OptionCallBack != nil {
				if err = option.OptionCallBack(preCtx, &option); err != nil {
					return
				}
			}
			resp, err = obj.request(preCtx, option)
			if err != nil { //有错误
				if errors.Is(err, ErrFatal) { //致命错误直接返回
					return
				} else if option.ErrCallBack != nil && option.ErrCallBack(preCtx, err) != nil { //不是致命错误，有错误回调,有错误,直接返回
					return
				}
			} else if option.ResultCallBack == nil { //没有错误，且没有回调，直接返回
				return
			} else if err = option.ResultCallBack(preCtx, resp); err != nil { //没有错误，有回调，回调错误
				if option.ErrCallBack != nil && option.ErrCallBack(preCtx, err) != nil { //有错误回调,有错误直接返回
					return
				}
			} else { //没有错误，有回调，没有回调错误，直接返回
				return
			}
		}
	}
	if err != nil { //有错误直接返回错误
		return
	}
	return resp, errors.New("max try num")
}
func verifyProxy(proxyUrl string) (*url.URL, error) {
	proxy, err := url.Parse(proxyUrl)
	if err != nil {
		return nil, err
	}
	switch proxy.Scheme {
	case "http", "socks5", "https":
		return proxy, nil
	default:
		return nil, tools.WrapError(ErrFatal, "不支持的代理协议")
	}
}
func (obj *Client) request(preCtx context.Context, option RequestOption) (response *Response, err error) {
	if err = option.optionInit(); err != nil {
		err = tools.WrapError(err, "option 初始化错误")
		return
	}
	method := strings.ToUpper(option.Method)
	href := option.converUrl
	var reqs *http.Request
	//构造ctxData
	ctxData := new(reqCtxData)
	ctxData.requestCallBack = option.RequestCallBack
	ctxData.responseCallBack = option.ResponseCallBack
	if option.Body != nil {
		ctxData.disBody = true
	}
	ctxData.disProxy = option.DisProxy
	if option.Proxy != "" { //代理相关构造
		tempProxy, err := verifyProxy(option.Proxy)
		if err != nil {
			return response, tools.WrapError(ErrFatal, errors.New("tempRequest 构造代理失败"), err)
		}
		ctxData.proxy = tempProxy
	} else if tempProxy := obj.dialer.Proxy(); tempProxy != nil {
		ctxData.proxy = tempProxy
	}
	if option.RedirectNum != 0 { //重定向次数
		ctxData.redirectNum = option.RedirectNum
	}
	//构造ctx,cnl
	var cancel context.CancelFunc
	var reqCtx context.Context
	if option.Timeout > 0 { //超时
		reqCtx, cancel = context.WithTimeout(context.WithValue(preCtx, keyPrincipalID, ctxData), option.Timeout)
	} else {
		reqCtx, cancel = context.WithCancel(context.WithValue(preCtx, keyPrincipalID, ctxData))
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
	if option.body != nil {
		reqs, err = http.NewRequestWithContext(reqCtx, method, href, option.body)
	} else {
		reqs, err = http.NewRequestWithContext(reqCtx, method, href, nil)
	}
	if err != nil {
		return response, tools.WrapError(ErrFatal, errors.New("tempRequest 构造request失败"), err)
	}
	ctxData.url = reqs.URL
	ctxData.host = reqs.Host
	if reqs.URL.Scheme == "file" {
		filePath := re.Sub(`^/+`, "", reqs.URL.Path)
		fileContent, err := os.ReadFile(filePath)
		if err != nil {
			return response, tools.WrapError(ErrFatal, errors.New("read filePath data error"), err)
		}
		cancel()
		return &Response{
			content:  fileContent,
			filePath: filePath,
		}, nil
	}
	//判断ws
	switch reqs.URL.Scheme {
	case "ws":
		ctxData.ws = true
		reqs.URL.Scheme = "http"
	case "wss":
		ctxData.ws = true
		reqs.URL.Scheme = "https"
	}
	//添加headers
	var headOk bool
	if reqs.Header, headOk = option.Headers.(http.Header); !headOk {
		return response, tools.WrapError(ErrFatal, "request headers 转换错误")
	}

	if reqs.Header.Get("Content-Type") == "" && reqs.Header.Get("content-type") == "" && option.ContentType != "" {
		reqs.Header.Set("Content-Type", option.ContentType)
	}
	//host构造
	if option.Host != "" {
		reqs.Host = option.Host
	} else if reqs.Header.Get("Host") != "" {
		reqs.Host = reqs.Header.Get("Host")
	}
	//添加cookies
	if option.Cookies != nil {
		cooks, cookOk := option.Cookies.(Cookies)
		if !cookOk {
			return response, tools.WrapError(ErrFatal, "request cookies 转换错误")
		}
		for _, vv := range cooks {
			reqs.AddCookie(vv)
		}
	}
	//开始发送请求
	var r *http.Response
	var err2 error
	if ctxData.ws {
		websocket.SetClientHeaders(reqs.Header, option.WsOption)
	}
	r, err = obj.getClient(option).Do(reqs)
	if r != nil {
		isSse := r.Header.Get("Content-Type") == "text/event-stream"

		if ctxData.responseCallBack != nil {
			var resp *ResponseDebug
			if resp, err = cloneResponse(r, isSse || ctxData.ws); err != nil {
				return
			}
			if err = ctxData.responseCallBack(reqCtx, resp); err != nil {
				return response, tools.WrapError(ErrFatal, "request requestCallBack 回调错误")
			}
		}
		if ctxData.ws {
			if r.StatusCode == 101 {
				option.DisRead = true
			} else if err == nil {
				err = errors.New("statusCode not 101")
			}
		} else if isSse {
			option.DisRead = true
		}
		if response, err2 = obj.newResponse(reqCtx, cancel, r, option); err2 != nil { //创建 response
			return response, err2
		}
		if ctxData.ws && r.StatusCode == 101 {
			if response.webSocket, err2 = websocket.NewClientConn(r); err2 != nil { //创建 websocket
				return response, err2
			}
		}
	}
	return response, err
}
