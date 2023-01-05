package redis

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"gitee.com/baixudong/gospider/tools"
	"github.com/go-redis/redis"
	"golang.org/x/exp/slices"
)

type Client struct {
	object *redis.Client
	proxys map[string][]string
	sync.Mutex
}
type ClientOption struct {
	Host string //host
	Port int    //端口号
	Pwd  string //密码
	Db   int    //数据库
}

func NewClient(options ...ClientOption) (*Client, error) {
	var option ClientOption
	if len(options) > 0 {
		option = options[0]
	}
	if option.Host == "" {
		option.Host = "localhost"
	}
	if option.Port == 0 {
		option.Port = 6379
	}
	redCli := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", option.Host, option.Port),
		DB:       option.Db,
		Password: option.Pwd,
	})
	_, err := redCli.Ping().Result()
	return &Client{object: redCli, proxys: make(map[string][]string)}, err
}

// 集合增加元素
func (r *Client) SAdd(name string, vals ...any) (int64, error) {
	return r.object.SAdd(name, vals...).Result()
}

// 判断元素是否存在集合
func (r *Client) SExists(name string, val any) (bool, error) {
	return r.object.SIsMember(name, val).Result()
}

// 集合长度
func (r *Client) SLen(name string) (int64, error) {
	return r.object.SCard(name).Result()
}

// 集合所有的值
func (r *Client) SVals(name string) ([]string, error) {
	return r.object.SMembers(name).Result()
}

// 删除一个元素返回
func (r *Client) SPop(name string) (string, error) {
	return r.object.SPop(name).Result()
}

// 删除元素
func (r *Client) SRem(name string, vals ...any) (int64, error) {
	return r.object.SRem(name, vals...).Result()
}

// 获取字典中的key值
func (r *Client) HGet(name string, key string) (string, error) {
	cmd := r.object.HGet(name, key)
	return cmd.Result()
}

// 获取字典
func (r *Client) HAll(name string) (map[string]string, error) {
	cmd := r.object.HGetAll(name)
	return cmd.Result()
}

// 获取字典所有key
func (r *Client) HKeys(name string) ([]string, error) {
	cmd := r.object.HKeys(name)
	return cmd.Result()
}

// 获取字典所有值
func (r *Client) HVals(name string) ([]string, error) {
	cmd := r.object.HVals(name)
	return cmd.Result()
}

// 获取字典长度
func (r *Client) HLen(name string) (int64, error) {
	cmd := r.object.HLen(name)
	return cmd.Result()
}

// 设置字典的值
func (r *Client) HSet(name string, key string, val string) (bool, error) {
	return r.object.HSet(name, key, val).Result()
}

// 删除字典的值
func (r *Client) HDel(name string, key string) (int64, error) {
	return r.object.HDel(name, key).Result()
}

// 关闭客户端
func (r *Client) Close() error {
	return r.object.Close()
}

// 随机获取代理
func (r *Client) GetProxy(key string) (string, error) {
	vals, err := r.GetProxys(key)
	if err != nil {
		return "", err
	}
	valLen := len(vals)
	if valLen == 0 {
		return "", errors.New("proxy enpty")
	}
	return vals[0], nil
}

// 随机获取代理(有序)
func (r *Client) GetOrderProxy(key string) (string, error) {
	vals, err := r.GetOrderProxys(key)
	if err != nil {
		return "", err
	}
	valLen := len(vals)
	if valLen == 0 {
		return "", errors.New("proxy enpty")
	}
	return vals[0], nil
}

type proxy struct {
	ip  string
	ttl int64
}

// 获取所有代理
func (r *Client) GetProxys(key string) ([]string, error) {
	vals, err := r.HVals(key)
	if err != nil {
		return nil, err
	}
	valLen := len(vals)
	if valLen == 0 {
		return nil, errors.New("代理为空")
	}
	proxys := []proxy{}
	for _, jsonStr := range vals {
		val := tools.Any2json(jsonStr)
		var ip, usr, pwd string
		var port int64
		if ip = val.Get("ip").String(); ip == "" {
			continue
		}
		if port = val.Get("port").Int(); port == 0 {
			continue
		}
		usr = val.Get("usr").String()
		pwd = val.Get("pwd").String()
		ttl := val.Get("ttl").Int()
		if usr != "" && pwd != "" {
			proxys = append(proxys,
				proxy{
					ip:  fmt.Sprintf("%s:%s@%s:%d", usr, pwd, ip, port),
					ttl: ttl,
				},
			)
		} else {
			proxys = append(proxys,
				proxy{
					ip:  fmt.Sprintf("%s:%d", ip, port),
					ttl: ttl,
				},
			)
		}
	}
	sort.Slice(proxys, func(i, j int) bool {
		return proxys[i].ttl > proxys[j].ttl
	})
	results := []string{}
	for _, proxy := range proxys {
		results = append(results, proxy.ip)
	}
	return results, nil
}

// 获取所有代理,排序后的
func (r *Client) GetOrderProxys(key string) ([]string, error) {
	proxys, err := r.GetProxys(key)
	if err != nil {
		return proxys, err
	}
	total := len(proxys)
	results := make([]string, total)
	orderProxy, ok := r.proxys[key]
	if !ok {
		r.proxys[key] = proxys
		return proxys, nil
	}
	newProxys := []string{}
	for _, val := range proxys {
		index := slices.Index(orderProxy, val)
		if index < total && index != -1 {
			results[index] = val
		} else {
			newProxys = append(newProxys, val)
		}
	}
	j := 0
	for i, reslut := range results {
		if reslut == "" {
			results[i] = newProxys[j]
			j++
		}
	}
	r.Lock()
	defer r.Unlock()
	r.proxys[key] = results
	return results, nil
}
