package re

import "regexp"

type ReData struct {
	data []string
}

//返回分组的匹配
func (obj *ReData) Group(nums ...int) string {
	var num int
	if len(nums) > 0 {
		num = nums[0]
	}
	return obj.data[num]
}

func compile(reg string) (*regexp.Regexp, error) {
	return regexp.Compile(reg)
}

//搜索
func Search(reg string, txt string) *ReData {
	comReg, err := compile(reg)
	if err != nil {
		return nil
	}
	data := comReg.FindStringSubmatch(txt)
	if len(data) == 0 {
		return nil
	}
	return &ReData{data: data}
}

//find 所有
func FindAll(reg string, txt string) []*ReData {
	datas := []*ReData{}
	comReg, err := compile(reg)
	if err != nil {
		return nil
	}
	results := comReg.FindAllStringSubmatch(txt, -1)
	for _, result := range results {
		datas = append(datas, &ReData{data: result})
	}
	return datas
}

//替换匹配
func Sub(reg string, rep string, txt string) string {
	comReg, err := compile(reg)
	if err != nil {
		return txt
	}
	return comReg.ReplaceAllString(txt, rep)
}

//使用方法替换匹配
func SubFunc(reg string, rep func(string) string, txt string) string {
	comReg, err := compile(reg)
	if err != nil {
		return txt
	}
	return comReg.ReplaceAllStringFunc(txt, rep)
}

//分割
func Split(reg string, txt string) []string {
	comReg, err := compile(reg)
	if err != nil {
		return nil
	}
	return comReg.Split(txt, -1)
}

//转义
func Quote(reg string) string {
	return regexp.QuoteMeta(reg)
}
