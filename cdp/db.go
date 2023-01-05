package cdp

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"gitee.com/baixudong/gospider/re"
	"gitee.com/baixudong/gospider/tools"
	"golang.org/x/exp/maps"

	"github.com/xujiajun/nutsdb"
)

type DbClient struct {
	db   *nutsdb.DB
	name string
}

func (obj *DbClient) Close() error {
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
func (obj *DbClient) put(key [16]byte, val any, ttl uint32) error {
	return obj.db.Update(
		func(tx *nutsdb.Tx) error {
			valNetwork := bytes.NewBuffer(nil)
			if err := gob.NewEncoder(valNetwork).Encode(val); err != nil {
				return err
			}
			return tx.Put(obj.name, key[:], valNetwork.Bytes(), ttl)
		})
}
func (obj *DbClient) get(key [16]byte, val any) error {
	return obj.db.View(
		func(tx *nutsdb.Tx) error {
			e, err := tx.Get(obj.name, key[:])
			if err != nil {
				return err
			}
			if e == nil {
				return errors.New("val is nil")
			}
			return gob.NewDecoder(bytes.NewReader(e.Value)).Decode(val)
		})
}
func NewDbClient(dir string) (*DbClient, error) {
	option := nutsdb.DefaultOptions
	option.EntryIdxMode = nutsdb.HintKeyAndRAMIdxMode
	db, err := nutsdb.Open(
		option,
		nutsdb.WithDir(dir),
	)
	return &DbClient{db: db, name: "goSpider"}, err
}
