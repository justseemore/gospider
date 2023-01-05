package tree

import (
	"sort"
	"sync"

	"gitee.com/baixudong/gospider/kinds"
)

type Client struct {
	dataStr      map[string]*kinds.Set[string]
	dataLen      map[string]*kinds.Set[int]
	dataOrdLen   map[string][]int
	dataSortKeys *kinds.Set[string]

	minNum int
	sync.Mutex
}
type ClientOption struct {
	MinNum int
}

func NewClient(options ...ClientOption) *Client {
	var option ClientOption
	if len(options) > 0 {
		option = options[0]
	}
	if option.MinNum == 0 {
		option.MinNum = 1
	}
	return &Client{
		dataStr:      map[string]*kinds.Set[string]{},
		dataLen:      map[string]*kinds.Set[int]{},
		dataOrdLen:   map[string][]int{},
		dataSortKeys: kinds.NewSet[string](),
		minNum:       option.MinNum,
	}
}
func (obj *Client) Add(words string) {
	wordrunes := []rune(words)
	if len(wordrunes) < obj.minNum {
		return
	}
	word_one := string(wordrunes[:obj.minNum])
	wordrune_str := wordrunes[obj.minNum:]
	word_str := string(wordrune_str)
	word_len := len(wordrune_str)

	value, ok := obj.dataStr[word_one]
	if ok {
		value.Add(word_str)
	} else {
		obj.dataStr[word_one] = kinds.NewSet(word_str)
	}
	value2, ok := obj.dataLen[word_one]
	if ok {
		if value2.Add(word_len) {
			obj.dataSortKeys.Add(word_one)
		}
	} else {
		obj.dataLen[word_one] = kinds.NewSet(word_len)
		obj.dataSortKeys.Add(word_one)
	}
}
func (obj *Client) sort() {
	obj.Lock()
	defer obj.Unlock()
	if obj.dataSortKeys.Len() == 0 {
		return
	}
	for _, k := range obj.dataSortKeys.Array() {
		obj.dataOrdLen[k] = make([]int, obj.dataLen[k].Len())
		for i, vv := range obj.dataLen[k].Array() {
			obj.dataOrdLen[k][i] = vv
		}
		sort.Ints(obj.dataOrdLen[k])
	}
	obj.dataSortKeys.ReSet()
}
func (obj *Client) Search(wordstr string) map[string]int {
	obj.sort()
	words := []rune(wordstr)
	search_dic := map[string]int{}
	words_len := len(words)
	if len(words) < obj.minNum {
		return search_dic
	}
	word_start := 0
	word_end := word_start + obj.minNum
	for word_end <= words_len { //排除完的长度<总长度
		word := string(words[word_start:word_end])
		value2, ok := obj.dataOrdLen[word]
		if ok {
			last_len := words_len - word_end //剩余长度=总长度减去-查询过的长度
			for word_len_index := len(value2) - 1; word_len_index >= 0; word_len_index-- {
				word_len := value2[word_len_index]
				if word_len > last_len {
					continue
				}
				qg_str := string(words[word_end : word_end+word_len])
				value := obj.dataStr[word]
				is_in := value.Has(qg_str)
				if is_in {
					search_value, search_ok := search_dic[word+qg_str]
					if search_ok {
						search_dic[word+qg_str] = search_value + 1
					} else {
						search_dic[word+qg_str] = 1
					}
					word_start += word_len
					break
				}
			}
		}
		word_start++
		word_end = word_start + obj.minNum
	}
	return search_dic
}
