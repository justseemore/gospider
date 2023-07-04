package requests

import (
	"errors"
	"net/http"

	"gitee.com/baixudong/gospider/tools"
	"github.com/tidwall/gjson"
)

var UserAgent = `Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/112.0.0.0 Safari/537.36`
var AcceptLanguage = `"zh-CN,zh;q=0.9"`

// 请求操作========================================================================= start
func DefaultHeaders() http.Header {
	return http.Header{
		"Accept-Encoding": []string{"gzip, deflate, br"},
		"Accept":          []string{"text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8"},
		"Accept-Language": []string{AcceptLanguage},
		"User-Agent":      []string{UserAgent},
	}
}
func (obj *RequestOption) newHeaders() error {
	if obj.Headers == nil {
		obj.Headers = DefaultHeaders()
		return nil
	}
	switch headers := obj.Headers.(type) {
	case http.Header:
		obj.Headers = headers.Clone()
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
