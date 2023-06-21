package requests

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"net/url"
	"strings"

	"gitee.com/baixudong/gospider/websocket"
)

// 请求参数选项
type RequestOption struct {
	Method        string   //method
	Url           *url.URL //请求的url
	Host          string   //网站的host
	Proxy         string   //代理,支持http,https,socks5协议代理,例如：http://127.0.0.1:7005
	Timeout       int64    //请求超时时间
	Headers       any      //请求头,支持：json,map，header
	Cookies       any      // cookies,支持json,map,str，http.Header
	Files         []File   //文件
	Params        any      //url 中的参数，用以拼接url,支持json,map
	Form          any      //发送multipart/form-data,适用于文件上传,支持json,map
	Data          any      //发送application/x-www-form-urlencoded,适用于key,val,支持string,[]bytes,json,map
	body          io.Reader
	Body          io.Reader
	Json          any                                         //发送application/json,支持：string,[]bytes,json,map
	Text          any                                         //发送text/xml,支持string,[]bytes,json,map
	ContentType   string                                      //headers 中Content-Type 的值
	Raw           any                                         //不设置context-type,支持string,[]bytes,json,map
	TempData      map[string]any                              //临时变量，用于回调存储或自由度更高的用法
	DisCookie     bool                                        //关闭cookies管理,这个请求不用cookies池
	DisDecode     bool                                        //关闭自动解码
	Bar           bool                                        //是否开启bar
	DisProxy      bool                                        //是否关闭代理,强制关闭代理
	TryNum        int64                                       //重试次数
	BeforCallBack func(context.Context, *RequestOption) error //请求之前回调
	AfterCallBack func(context.Context, *Response) error      //请求之后回调
	Jar           *Jar                                        //自定义临时cookies 管理

	ErrCallBack func(context.Context, error) bool //返回true 中断重试请求
	RedirectNum int                               //重定向次数,小于零 关闭重定向
	DisRead     bool                              //关闭默认读取请求体,不会主动读取body里面的内容，需用你自己读取
	DisUnZip    bool                              //关闭自动解压
	WsOption    websocket.Option                  //websocket option,使用websocket 请求的option

	converUrl string
}

func (obj *RequestOption) optionInit() error {
	obj.converUrl = obj.Url.String()
	var err error
	//构造body
	if obj.Raw != nil {
		if obj.body, err = newBody(obj.Raw, "raw", nil); err != nil {
			return err
		}
	} else if obj.Form != nil {
		dataMap := map[string][]string{}
		if obj.body, err = newBody(obj.Form, "form", dataMap); err != nil {
			return err
		}
		tempBody := bytes.NewBuffer(nil)
		writer := multipart.NewWriter(tempBody)
		for key, vals := range dataMap {
			for _, val := range vals {
				if err = writer.WriteField(key, val); err != nil {
					return err
				}
			}
		}
		escapeQuotes := strings.NewReplacer("\\", "\\\\", `"`, "\\\"")
		for _, file := range obj.Files {
			h := make(textproto.MIMEHeader)
			h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, escapeQuotes.Replace(file.Key), escapeQuotes.Replace(file.Name)))
			if file.Type == "" {
				h.Set("Content-Type", "application/octet-stream")
			} else {
				h.Set("Content-Type", file.Type)
			}
			if wp, err := writer.CreatePart(h); err != nil {
				return err
			} else if _, err = wp.Write(file.Content); err != nil {
				return err
			}
		}
		if err = writer.Close(); err != nil {
			return err
		}
		if obj.ContentType == "" {
			obj.ContentType = writer.FormDataContentType()
		}
		obj.body = tempBody
	} else if obj.Data != nil {
		if obj.body, err = newBody(obj.Data, "data", nil); err != nil {
			return err
		}
		if obj.ContentType == "" {
			obj.ContentType = "application/x-www-form-urlencoded"
		}
	} else if obj.Json != nil {
		if obj.body, err = newBody(obj.Json, "json", nil); err != nil {
			return err
		}
		if obj.ContentType == "" {
			obj.ContentType = "application/json"
		}
	} else if obj.Text != nil {
		if obj.body, err = newBody(obj.Text, "text", nil); err != nil {
			return err
		}
		if obj.ContentType == "" {
			obj.ContentType = "text/plain"
		}
	}
	//构造params
	if obj.Params != nil {
		dataMap := map[string][]string{}
		if _, err = newBody(obj.Params, "params", dataMap); err != nil {
			return err
		}
		pu := cloneUrl(obj.Url)
		puValues := pu.Query()
		for kk, vvs := range dataMap {
			for _, vv := range vvs {
				puValues.Add(kk, vv)
			}
		}
		pu.RawQuery = puValues.Encode()
		obj.converUrl = pu.String()
	}
	//构造headers
	if err = obj.newHeaders(); err != nil {
		return err
	}
	//构造cookies
	return obj.newCookies()
}
func (obj *Client) newRequestOption(option RequestOption) (RequestOption, error) {
	if option.TryNum == 0 {
		option.TryNum = obj.tryNum
	}
	if option.BeforCallBack == nil {
		option.BeforCallBack = obj.beforCallBack
	}
	if option.AfterCallBack == nil {
		option.AfterCallBack = obj.afterCallBack
	}
	if option.ErrCallBack == nil {
		option.ErrCallBack = obj.errCallBack
	}
	if option.Proxy == "" {
		option.Proxy = obj.proxy
	}
	if option.Headers == nil {
		if obj.headers == nil {
			option.Headers = DefaultHeaders.Clone()
		} else {
			option.Headers = obj.headers
		}
	}
	if !option.Bar {
		option.Bar = obj.bar
	}
	if option.RedirectNum == 0 {
		option.RedirectNum = obj.redirectNum
	}
	if option.Timeout == 0 {
		option.Timeout = obj.timeout
	}
	if !option.DisCookie {
		option.DisCookie = obj.disCookie
	}
	if !option.DisDecode {
		option.DisDecode = obj.disDecode
	}
	if !option.DisRead {
		option.DisRead = obj.disRead
	}
	if !option.DisUnZip {
		option.DisUnZip = obj.disUnZip
	}
	var err error
	if con, ok := option.Json.(io.Reader); ok {
		if option.Json, err = io.ReadAll(con); err != nil {
			return option, err
		}
	}
	if con, ok := option.Text.(io.Reader); ok {
		if option.Text, err = io.ReadAll(con); err != nil {
			return option, err
		}
	}
	if con, ok := option.Data.(io.Reader); ok {
		if option.Data, err = io.ReadAll(con); err != nil {
			return option, err
		}
	}
	return option, err
}
