package cdp

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"sync"
	"time"

	"gitee.com/baixudong/gospider/chanx"
	"gitee.com/baixudong/gospider/re"
	"gitee.com/baixudong/gospider/tools"
	"golang.org/x/exp/maps"

	"github.com/syndtr/goleveldb/leveldb"
)

type DbClient struct {
	db       *leveldb.DB
	orderKey *chanx.Client[dbKey]
	mapKey   sync.Map
	timeOut  int64
	ctx      context.Context
	cnl      context.CancelFunc
}
type dbKey struct {
	key [16]byte
	ttl int64
}

func NewDbClient(preCtx context.Context, dir string) (*DbClient, error) {
	ctx, cnl := context.WithCancel(preCtx)
	db, err := leveldb.OpenFile(dir, nil)
	client := &DbClient{
		ctx:      ctx,
		cnl:      cnl,
		db:       db,
		timeOut:  60 * 60,
		orderKey: chanx.NewClient[dbKey](ctx),
	}
	go client.run()
	return client, err
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
			mapValAny, ok := obj.mapKey.Load(orderVal.key)
			if ok { //删除mapkey，删除db 数据,数据过期开始删除
				if mapVal, ok := mapValAny.(int64); ok && (orderVal.ttl == mapVal || time.Now().Unix()-mapVal >= obj.timeOut) {
					obj.mapKey.Delete(orderVal.key)
					obj.db.Delete(orderVal.key[:], nil)
				}
			}
		}
	}
}
func (obj *DbClient) Close() error {
	obj.cnl()
	return obj.db.Close()
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
	valNetwork := bytes.NewBuffer(nil)
	if err := gob.NewEncoder(valNetwork).Encode(fulData); err != nil {
		return err
	} else if err = obj.db.Put(key[:], valNetwork.Bytes(), nil); err != nil {
		return err
	}
	nowTime := time.Now().Unix()
	obj.orderKey.Add(dbKey{key: key, ttl: nowTime})
	obj.mapKey.Store(key, nowTime)
	return nil
}
func (obj *DbClient) get(key [16]byte) (fulData FulData, err error) {
	var con []byte
	if con, err = obj.db.Get(key[:], nil); err != nil {
		if !errors.Is(err, leveldb.ErrNotFound) {
			log.Print(err)
		}
		return
	}
	err = gob.NewDecoder(bytes.NewReader(con)).Decode(&fulData)
	return
}
