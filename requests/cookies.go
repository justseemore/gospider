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
		cookies := Cookies{}
		for kk, vvs := range cook.Map() {
			if vvs.IsArray() {
				for _, vv := range vvs.Array() {
					cookies = append(cookies, &http.Cookie{
						Name:  kk,
						Value: vv.String(),
					})
				}
			} else {
				cookies = append(cookies, &http.Cookie{
					Name:  kk,
					Value: vvs.String(),
				})
			}
		}
		return cookies, nil
	default:
		jsonData := tools.Any2json(cook)
		if !jsonData.IsObject() {
			return nil, errors.New("cookies不支持的类型")
		}

		cookies := Cookies{}
		for kk, vvs := range jsonData.Map() {
			if vvs.IsArray() {
				for _, vv := range vvs.Array() {
					cookies = append(cookies, &http.Cookie{
						Name:  kk,
						Value: vv.String(),
					})
				}
			} else {
				cookies = append(cookies, &http.Cookie{
					Name:  kk,
					Value: vvs.String(),
				})
			}
		}
		return cookies, nil
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
		cookies := Cookies{}
		for kk, vvs := range cook.Map() {
			if vvs.IsArray() {
				for _, vv := range vvs.Array() {
					cookies = append(cookies, &http.Cookie{
						Name:  kk,
						Value: vv.String(),
					})
				}
			} else {
				cookies = append(cookies, &http.Cookie{
					Name:  kk,
					Value: vvs.String(),
				})
			}
		}
		return cookies, nil
	default:
		jsonData := tools.Any2json(cook)
		if !jsonData.IsObject() {
			return nil, errors.New("setCookies 不支持的类型")
		}
		cookies := Cookies{}
		for kk, vvs := range jsonData.Map() {
			if vvs.IsArray() {
				for _, vv := range vvs.Array() {
					cookies = append(cookies, &http.Cookie{
						Name:  kk,
						Value: vv.String(),
					})
				}
			} else {
				cookies = append(cookies, &http.Cookie{
					Name:  kk,
					Value: vvs.String(),
				})
			}
		}
		return cookies, nil
	}
}
func (obj *RequestOption) newCookies() (err error) {
	if obj.Cookies == nil {
		return nil
	}
	obj.Cookies, err = ReadCookies(obj.Cookies)
	return err
}
