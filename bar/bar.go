package bar

import (
	"bytes"
	"fmt"
	"math"
	"sync"
	"time"

	"gitee.com/baixudong/gospider/blog"
)

type Client struct {
	cur       int64         //当前数量
	total     int64         //总数量
	shape     string        //进度条图像
	percent   int           //进度
	rate      *bytes.Buffer //进度条
	lock      sync.Mutex
	startTime int64
	preMt     barMt
	preMt2    barMt
}
type ClientOption struct {
	Cur   int64  //当前数量
	Shape string //进度条图像
}

func time2Dhms(val int64) string {
	rs := ""
	year := val / (60 * 60 * 24 * 365)
	if year > 0 {
		val = val % (60 * 60 * 24 * 365)
	}
	day := val / (60 * 60 * 24) //天
	if day > 0 {
		val = val % (60 * 60 * 24)
	}
	hour := val / (60 * 60) //时
	if hour > 0 {
		val = val % (60 * 60)
	}
	mint := val / 60 //分
	sencod := val % 60

	if sencod > 0 {
		rs = fmt.Sprintf("%ds", sencod) + rs
	}
	if mint > 0 {
		rs = fmt.Sprintf("%dm", mint) + rs
	}
	if hour > 0 {
		rs = fmt.Sprintf("%dh", hour) + rs
	}
	if day > 0 {
		rs = fmt.Sprintf("%dd", day) + rs
	}
	if year > 0 {
		rs = fmt.Sprintf("%dy", year) + rs
	}
	if rs == "" {
		return "0s"
	}
	return rs
}
func NewClient(total int64, options ...ClientOption) *Client {
	var option ClientOption
	if len(options) > 0 {
		option = options[0]
	}
	if option.Shape == "" {
		option.Shape = "━"
	}
	bar := &Client{
		total: total,
		cur:   option.Cur,
		shape: option.Shape,
		rate:  bytes.NewBuffer(nil),
		preMt: barMt{
			time: time.Now().Unix(),
			cur:  option.Cur,
		},
	}
	bar.startTime = bar.preMt.time
	bar.preMt2 = bar.preMt
	return bar
}

type barMt struct {
	time int64
	cur  int64
}

// 打印进度条
func (obj *Client) Print(curs ...int64) {
	obj.lock.Lock()
	defer obj.lock.Unlock()

	if len(curs) == 0 {
		obj.cur++
	} else {
		obj.cur += curs[0]
	}
	nowMt := barMt{
		time: time.Now().Unix(),
		cur:  obj.cur,
	}
	percent := int(nowMt.cur * 100 / obj.total)
	for i := 0; i < percent-obj.percent; i++ {
		if (percent+i)%2 == 0 {
			obj.rate.WriteString(obj.shape)
		}
	}
	obj.percent = percent
	sn := time2Dhms(nowMt.time - obj.startTime) //已运行时间
	nt := float64(nowMt.time - obj.preMt.time)
	var mt float64
	if nt == 0 {
		mt = float64(nowMt.cur - obj.preMt.cur)
	} else {
		mt = float64(nowMt.cur-obj.preMt.cur) / nt
	}
	se := time2Dhms(int64(math.Ceil(float64(obj.total-obj.cur) / mt))) //剩余时间
	fmt.Println(
		blog.Color(5, 0, time.Now().Format(time.DateTime)),
		blog.Color(2, 0, fmt.Sprintf(" %-50s", obj.rate.String())),
		blog.Color(4, 0, fmt.Sprintf(" %3d%%", obj.percent)),
		blog.Color(3, 0, fmt.Sprintf(" %d/%d", obj.cur, obj.total)),
		blog.Color(6, 0, fmt.Sprintf(" %s<%s", sn, se)),
		blog.Color(1, 0, fmt.Sprintf(" %0.2f/s", mt)),
	)
	if nowMt.time-obj.preMt2.time > 300 {
		obj.preMt, obj.preMt2 = obj.preMt2, nowMt
	}
}
