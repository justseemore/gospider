package requests

import (
	"bytes"
	"errors"
	"net/url"

	"gitee.com/baixudong/gospider/tools"
	"github.com/tidwall/gjson"
)

// 构造一个文件
type File struct {
	Key     string //字段的key
	Name    string //文件名
	Content []byte //文件的内容
	Type    string //文件类型
}

func newBody(val any, valType string, dataMap map[string][]string) (*bytes.Reader, error) {
	switch value := val.(type) {
	case gjson.Result:
		if !value.IsObject() {
			return nil, errors.New("body-type错误")
		}
		switch valType {
		case "json", "text", "raw":
			return bytes.NewReader(tools.StringToBytes(value.Raw)), nil
		case "data":
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
		case "form", "params":
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
		case "json", "text", "data", "raw":
			return bytes.NewReader(tools.StringToBytes(value)), nil
		default:
			return nil, errors.New("未知的content-type：" + valType)
		}
	case []byte:
		switch valType {
		case "json", "text", "data", "raw":
			return bytes.NewReader(value), nil
		default:
			return nil, errors.New("未知的content-type：" + valType)
		}
	default:
		result, err := tools.Any2json(value)
		if err != nil {
			return nil, err
		}
		return newBody(result, valType, dataMap)
	}
}
