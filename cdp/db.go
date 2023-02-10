package cdp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"gitee.com/baixudong/gospider/chanx"
	"gitee.com/baixudong/gospider/re"
	"gitee.com/baixudong/gospider/tools"
	"golang.org/x/exp/maps"
)

type DbClient struct {
	orderKey *chanx.Client[dbKey]
	mapKey   map[[16]byte]dbData
	lock     sync.RWMutex
	timeOut  int64
	ctx      context.Context
	cnl      context.CancelFunc
}
type dbKey struct {
	key [16]byte
	ttl int64
}
type dbData struct {
	data FulData
	ttl  int64
}

func NewDbClient(ctx context.Context, cnl context.CancelFunc) *DbClient {
	client := &DbClient{
		ctx:      ctx,
		cnl:      cnl,
		timeOut:  60 * 15,
		mapKey:   make(map[[16]byte]dbData),
		orderKey: chanx.NewClient[dbKey](ctx),
	}
	go client.run()
	return client
}

func (obj *DbClient) run() {
	defer obj.Close()
	for {
		select {
		case <-obj.ctx.Done():
			return
		case <-obj.orderKey.Done():
			return
		case orderVal := <-obj.orderKey.Chan():
			if awaitTime := obj.timeOut - (time.Now().Unix() - orderVal.ttl); awaitTime > 0 { //判断睡眠时间
				select {
				case <-obj.ctx.Done():
					return
				case <-obj.orderKey.Done():
					return
				case <-time.After(time.Second * time.Duration(awaitTime)):
				}
			}
			obj.lock.RLock()
			mapVal, ok := obj.mapKey[orderVal.key]
			obj.lock.RUnlock()
			if ok && (orderVal.ttl == mapVal.ttl || time.Now().Unix()-mapVal.ttl >= obj.timeOut) { //删除mapkey，删除db 数据,数据过期开始删除
				obj.lock.Lock()
				delete(obj.mapKey, orderVal.key)
				obj.lock.Unlock()
			}
		}
	}
}
func (obj *DbClient) Close() {
	obj.cnl()
}
func (obj *DbClient) keyMd5(key RequestOption, resourceType string) [16]byte {
	var md5Str string
	nt := strconv.Itoa(int(time.Now().Unix() / 1000))
	key.Url = re.Sub(fmt.Sprintf(`=%s\d*?&`, nt), "=&", key.Url)
	key.Url = re.Sub(fmt.Sprintf(`=%s\d*?$`, nt), "=", key.Url)

	key.Url = re.Sub(fmt.Sprintf(`=%s\d*?\.\d+?&`, nt), "=&", key.Url)
	key.Url = re.Sub(fmt.Sprintf(`=%s\d*?\.\d+?$`, nt), "=", key.Url)

	key.Url = re.Sub(`=0\.\d{10,}&`, "=&", key.Url)
	key.Url = re.Sub(`=0\.\d{10,}$`, "=", key.Url)

	md5Str += fmt.Sprintf("%s,%s,%s", key.Method, key.Url, key.PostData)

	if resourceType == "Document" || "resourceType" == "XHR" {
		kks := maps.Keys(key.Headers)
		sort.Strings(kks)
		for _, k := range kks {
			md5Str += fmt.Sprintf("%s,%s", k, key.Headers[k])
		}
	}
	return tools.Md5(md5Str)
}
func (obj *DbClient) put(key [16]byte, fulData FulData) error {
	nowTime := time.Now().Unix()
	obj.orderKey.Add(dbKey{key: key, ttl: nowTime})
	obj.lock.Lock()
	obj.mapKey[key] = dbData{data: fulData, ttl: nowTime}
	obj.lock.Unlock()
	return nil
}
func (obj *DbClient) get(key [16]byte) (fulData FulData, err error) {
	obj.lock.RLock()
	mapVal, ok := obj.mapKey[key]
	obj.lock.RUnlock()
	if ok {
		fulData = mapVal.data
	} else {
		err = errors.New("not found")
	}
	return
}
