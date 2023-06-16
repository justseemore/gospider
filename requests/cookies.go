package requests

import (
	"errors"
	"net/http"
	_ "unsafe"

	"gitee.com/baixudong/gospider/tools"
	"github.com/tidwall/gjson"
)

//go:linkname readCookies net/http.readCookies
func readCookies(h http.Header, filter string) []*http.Cookie

//go:linkname readSetCookies net/http.readSetCookies
func readSetCookies(h http.Header) []*http.Cookie

// 支持json,map,[]string,http.Header,string
func ReadCookies(val any) (Cookies, error) {
	switch cook := val.(type) {
	case Cookies:
		return cook, nil
	case []*http.Cookie:
		return Cookies(cook), nil
	case string:
		return readCookies(http.Header{"Cookie": []string{cook}}, ""), nil
	case http.Header:
		return readCookies(cook, ""), nil
	case []string:
		return readCookies(http.Header{"Cookie": cook}, ""), nil
	case gjson.Result:
		if !cook.IsObject() {
			return nil, errors.New("cookies不支持的类型")
		}
		head := http.Header{}
		for k, vvs := range cook.Map() {
			if vvs.IsArray() {
				for _, vv := range vvs.Array() {
					head.Add(k, vv.String())
				}
			} else {
				head.Add(k, vvs.String())
			}
		}
		return readCookies(head, ""), nil
	default:
		jsonData := tools.Any2json(cook)
		if !jsonData.IsObject() {
			return nil, errors.New("cookies不支持的类型")
		}
		head := http.Header{}
		for k, vvs := range jsonData.Map() {
			if vvs.IsArray() {
				for _, vv := range vvs.Array() {
					head.Add(k, vv.String())
				}
			} else {
				head.Add(k, vvs.String())
			}
		}
		return readCookies(head, ""), nil
	}
}

func ReadSetCookies(val any) (Cookies, error) {
	switch cook := val.(type) {
	case Cookies:
		return cook, nil
	case []*http.Cookie:
		return Cookies(cook), nil
	case string:
		return readSetCookies(http.Header{"Set-Cookie": []string{cook}}), nil
	case http.Header:
		return readSetCookies(cook), nil
	case []string:
		return readSetCookies(http.Header{"Set-Cookie": cook}), nil
	case gjson.Result:
		if !cook.IsObject() {
			return nil, errors.New("setCookies 不支持的类型")
		}
		head := http.Header{}
		for k, vvs := range cook.Map() {
			if vvs.IsArray() {
				for _, vv := range vvs.Array() {
					head.Add(k, vv.String())
				}
			} else {
				head.Add(k, vvs.String())
			}
		}
		return readSetCookies(head), nil
	default:
		jsonData := tools.Any2json(cook)
		if !jsonData.IsObject() {
			return nil, errors.New("setCookies 不支持的类型")
		}
		head := http.Header{}
		for k, vvs := range jsonData.Map() {
			if vvs.IsArray() {
				for _, vv := range vvs.Array() {
					head.Add(k, vv.String())
				}
			} else {
				head.Add(k, vvs.String())
			}
		}
		return readSetCookies(head), nil
	}
}
func (obj *RequestOption) newCookies() (err error) {
	if obj.Cookies == nil {
		return nil
	}
	obj.Cookies, err = ReadCookies(obj.Cookies)
	return err
}
