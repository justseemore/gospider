package city

import (
	"context"
	_ "embed"
	"encoding/json"
	"os"
	"sort"
	"strings"

	"github.com/justseemore/gospider/kinds"
	"github.com/justseemore/gospider/re"
	"github.com/justseemore/gospider/requests"
	"github.com/justseemore/gospider/tree"
)

func subProvince(province string) string {
	province = re.Sub(`市|省|\s`, "", province)
	if strings.Contains(province, "香港") {
		province = "香港"
	}
	if strings.Contains(province, "蒙古") {
		province = "蒙古"
	}
	if strings.Contains(province, "新疆") {
		province = "新疆"
	}
	if strings.Contains(province, "广西") {
		province = "广西"
	}
	if strings.Contains(province, "西藏") {
		province = "西藏"
	}
	if strings.Contains(province, "宁夏") {
		province = "宁夏"
	}
	if strings.Contains(province, "澳门") {
		province = "澳门"
	}
	return province
}
func subCity(city string) string {
	city = re.Sub(`\s`, "", city)
	city = re.Sub("(.{2,})[市县州旗镇乡岛]$", "${1}", city)
	return city
}
func subXian(xian string) string {
	xian = re.Sub(`\s`, "", xian)
	xian = re.Sub("(.{2,})新区$", "${1}", xian)
	xian = re.Sub("(.{2,})[区市县州旗镇乡岛]$", "${1}", xian)
	xian = re.Sub("(.{2,})?自治.*", "${1}", xian)
	xian = re.Sub(`[\(（].+?[\)）]$`, "", xian)
	return xian
}

type Province struct {
	Name     string `json:"name"`
	Value    int64  `json:"value"`
	subName  string
	Children []City `json:"children"`
}
type City struct {
	Name     string `json:"name"`
	subName  string
	Value    int64  `json:"value"`
	Children []Xian `json:"children"`
}
type Xian struct {
	Name    string `json:"name"`
	subName string
	Value   int64 `json:"value"`
}

func GetCity(pre_ctx context.Context, file_name string) error {
	main_url := "http://www.tcmap.com.cn/list/jiancheng_list.html"
	session, err := requests.NewClient(pre_ctx)
	if err != nil {
		return err
	}
	main_a, _ := session.Request(pre_ctx, "GET", main_url)
	main_html := main_a.Html()
	main_trs := main_html.Find("div[id=page_left] table").Finds("tr")
	var valueNum int64
	qcData := kinds.NewSet[string]()
	Provinces := []Province{}
	for _, main_tr := range main_trs {
		province := re.Sub(`\s`, "", main_tr.Finds("td")[1].Text())
		if province == "" {
			continue
		}
		valueNum++
		var provinceData Province
		provinceData.Name = province
		provinceData.Value = valueNum
		if qcData.Has(provinceData.Name) {
			continue
		} else {
			qcData.Add(provinceData.Name)
		}
		province_url := "http://www.tcmap.com.cn" + main_tr.Find("a").Get("href")
		province_a, _ := session.Request(pre_ctx, "GET", province_url)
		province_html := province_a.Html()
		province_trs := province_html.Find("div[id=page_left]>table").Finds("tr")
		Citys := []City{}
		for _, province_tr := range province_trs {
			province_tds := province_tr.Finds("td")
			city := re.Sub(`\s`, "", province_tds[0].Text())
			if city == "" {
				continue
			}
			valueNum++
			var cityData City
			cityData.Name = city
			cityData.Value = valueNum
			if qcData.Has(provinceData.Name + cityData.Name) {
				continue
			} else {
				qcData.Add(provinceData.Name + cityData.Name)
			}
			Xians := []Xian{}
			for _, xian := range strings.Split(province_tds[len(province_tds)-1].Text(), " ") {
				xian = re.Sub(`\s`, "", xian)
				if xian == "" {
					continue
				}
				valueNum++
				var xianData Xian
				xianData.Name = xian
				xianData.Value = valueNum
				if qcData.Has(provinceData.Name + cityData.Name + xianData.Name) {
					continue
				} else {
					qcData.Add(provinceData.Name + cityData.Name + xianData.Name)
				}
				Xians = append(Xians, xianData)
			}
			cityData.Children = Xians
			Citys = append(Citys, cityData)
		}
		provinceData.Children = Citys
		Provinces = append(Provinces, provinceData)
	}
	content, err := json.Marshal(Provinces)
	if err != nil {
		return err
	}
	return os.WriteFile(file_name, content, 0777)
}

type Client struct {
	tree   *tree.Client
	option ClientOption
}

//go:embed city.json
var city_content []byte

type ClientOption struct {
	Datas       []Province
	SubProvince bool
	SubCity     bool
	SubXian     bool
}

// 根据映射表创建客户端
func newClient(option ClientOption) *Client {
	city_tree := tree.NewClient()
	for provinceIndex, province := range option.Datas {
		city_tree.Add(province.Name)
		if option.SubProvince {
			province2 := subProvince(province.Name)
			if province2 != province.Name && province2 != "" {
				city_tree.Add(province2)
				option.Datas[provinceIndex].subName = province2
			}
		}
		for cityIndex, city := range province.Children {
			city_tree.Add(city.Name)
			if option.SubCity {
				city2 := subCity(city.Name)
				if city2 != city.Name && city2 != "" {
					city_tree.Add(city2)
					option.Datas[provinceIndex].Children[cityIndex].subName = city2
				}
			}
			for xianIndex, xian := range city.Children {
				city_tree.Add(xian.Name)
				if option.SubXian {
					xian2 := subXian(xian.Name)
					if xian2 != xian.Name && xian2 != "" {
						city_tree.Add(xian2)
						option.Datas[provinceIndex].Children[cityIndex].Children[xianIndex].subName = xian2
					}
				}
			}
		}
	}
	return &Client{option: option, tree: city_tree}
}

// 创建根据映射关系创建默认的客户端
func NewClient(options ...ClientOption) *Client {
	if len(options) > 0 {
		return newClient(options[0])
	}
	var data []Province
	err := json.Unmarshal(city_content, &data)
	if err != nil {
		return nil
	}
	return newClient(ClientOption{
		Datas:       data,
		SubProvince: true,
		SubCity:     true,
		SubXian:     true,
	})
}

func (obj *Client) getSearchData(searchData map[string]int) []*Node {
	if searchData == nil {
		return nil
	}
	results := []*Node{}
	for _, province := range obj.option.Datas {
		provinceCount := searchData[province.Name]
		provinceCount2 := searchData[province.subName]
		var haveCity bool
		var haveXian2 bool
		for _, city := range province.Children {
			cityCount := searchData[city.Name]
			cityCount2 := searchData[city.subName]
			if city.Name == province.Name {
				cityCount = 0
				cityCount, provinceCount = provinceCount, cityCount
			}
			if city.subName == province.subName {
				cityCount2 = 0
				cityCount2, provinceCount2 = provinceCount2, cityCount2
			}
			var haveXian bool
			for _, xian := range city.Children {
				xianCount := searchData[xian.Name]
				xianCount2 := searchData[xian.subName]
				if xian.Name == city.Name {
					xianCount = 0
					cityCount, xianCount = xianCount, cityCount
				}
				if xian.subName == city.subName {
					xianCount2 = 0
					cityCount2, xianCount2 = xianCount2, cityCount2
				}
				if xianCount+xianCount2 > 0 {
					haveXian = true
					haveXian2 = true
					results = append(results, &Node{
						Province: province.Name,
						City:     city.Name,
						Xian:     xian.Name,

						subProvince: province.subName,
						subCity:     city.subName,
						subXian:     xian.subName,

						ProvinceValue: province.Value,
						CityValue:     city.Value,
						XianValue:     xian.Value,

						provinceSize: provinceCount,
						citySize:     cityCount,
						xianSize:     xianCount,

						subProvinceSize: provinceCount2,
						subCitySize:     cityCount2,
						subXianSize:     xianCount2,
					})
				}
			}
			if !haveXian && cityCount+cityCount2 > 0 {
				haveCity = true
				results = append(results, &Node{
					Province: province.Name,
					City:     city.Name,

					subProvince: province.subName,
					subCity:     city.subName,

					ProvinceValue: province.Value,
					CityValue:     city.Value,

					provinceSize: provinceCount,
					citySize:     cityCount,

					subProvinceSize: provinceCount2,
					subCitySize:     cityCount2,
				})

			}
		}
		if !haveCity && !haveXian2 && provinceCount+provinceCount2 > 0 {
			haveCity = true
			results = append(results, &Node{
				subProvince: province.subName,
				Province:    province.Name,

				ProvinceValue:   province.Value,
				provinceSize:    provinceCount,
				subProvinceSize: provinceCount2,
			})
		}
	}
	sort.Slice(results, func(i, j int) bool {
		node := results[i]
		node2 := results[j]

		socre := node.score1()
		socre2 := node2.score1()
		if socre == socre2 {
			socre = node.score2()
			socre2 = node2.score2()
			if socre == socre2 {
				socre = node.score3()
				socre2 = node2.score3()
			}
		}
		return socre > socre2
	})
	return results
}

type Node struct {
	Province string //省
	City     string //市
	Xian     string //县

	subProvince string //省
	subCity     string //市
	subXian     string //县

	ProvinceValue int64
	CityValue     int64
	XianValue     int64

	provinceSize int
	citySize     int
	xianSize     int

	subProvinceSize int
	subCitySize     int
	subXianSize     int
}

func (obj Node) score1() int {
	if obj.provinceSize > 0 && obj.citySize > 0 && obj.xianSize > 0 {
		return 10
	}
	if obj.subProvinceSize > 0 && obj.subCitySize > 0 && obj.subXianSize > 0 {
		return 9
	}
	if obj.provinceSize > 0 && obj.citySize > 0 {
		return 8
	}
	if obj.subProvinceSize > 0 && obj.subCitySize > 0 {
		return 7
	}
	if obj.citySize > 0 && obj.xianSize > 0 {
		return 7
	}
	if obj.subCitySize > 0 && obj.subXianSize > 0 {
		return 6
	}

	if obj.provinceSize > 0 && obj.xianSize > 0 {
		return 6
	}
	if obj.subProvinceSize > 0 && obj.subXianSize > 0 {
		return 5
	}

	if obj.provinceSize > 0 {
		return 5
	}
	if obj.citySize > 0 {
		return 4
	}
	if obj.xianSize > 0 {
		return 3
	}

	if obj.subProvinceSize > 0 {
		return 4
	}
	if obj.subCitySize > 0 {
		return 3
	}
	if obj.subXianSize > 0 {
		return 2
	}
	return 0
}
func (obj Node) score2() int {
	return obj.provinceSize*7 + obj.citySize*3 + obj.xianSize
}
func (obj Node) score3() int {
	return obj.subProvinceSize*7 + obj.subCitySize*3 + obj.subXianSize
}

// 返回所有可能
func (obj *Client) Searchs(txt string) []*Node {
	return obj.getSearchData(obj.tree.Search(re.Sub(`\s|北京时间`, "", txt)))
}

// 返回分数最大的结果
func (obj *Client) Search(txt string) *Node {
	rs := obj.Searchs(txt)
	if len(rs) > 0 {
		return rs[0]
	} else {
		return nil
	}
}
