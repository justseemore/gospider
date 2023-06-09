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
func ReadCookies(val any) Cookies {
	switch cook := val.(type) {
	case string:
		return readCookies(http.Header{"Cookie": []string{cook}}, "")
	case http.Header:
		return readCookies(cook, "")
	case []string:
		return readCookies(http.Header{"Cookie": cook}, "")
	default:
		jsonData := tools.Any2json(cook)
		if jsonData.IsObject() {
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
			return readCookies(head, "")
		}
		return nil
	}
}

func ReadSetCookies(val any) Cookies {
	switch cook := val.(type) {
	case string:
		return readSetCookies(http.Header{"Set-Cookie": []string{cook}})
	case http.Header:
		return readSetCookies(cook)
	case []string:
		return readSetCookies(http.Header{"Set-Cookie": cook})
	default:
		jsonData := tools.Any2json(cook)
		if jsonData.IsObject() {
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
			return readSetCookies(head)
		}
		return nil
	}
}
func (obj *RequestOption) newCookies() error {
	if obj.Cookies == nil {
		return nil
	}
	switch cookies := obj.Cookies.(type) {
	case Cookies:
		return nil
	case []*http.Cookie:
		obj.Cookies = Cookies(cookies)
		return nil
	case string:
		obj.Cookies = ReadCookies(cookies)
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
