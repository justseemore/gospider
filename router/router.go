package router

import (
	"reflect"

	"github.com/gin-gonic/gin"
)

type Client struct {
	enGine *gin.Engine
}
type HandlerFunc struct {
	Method string
	Path   string          //路径
	Func   gin.HandlerFunc //方法
}

func (obj *Client) getHandlers(value any) []HandlerFunc {
	typeOfVal := reflect.TypeOf(value)
	results := []HandlerFunc{}
	for i := 0; i < typeOfVal.NumMethod(); i++ {
		fun := typeOfVal.Method(i)
		if fun.Type.NumIn() == 1 && fun.Type.In(0).String() == typeOfVal.String() && fun.Type.NumOut() == 1 && fun.Type.Out(0).String() == "router.HandlerFunc" {
			params := make([]reflect.Value, 1)
			params[0] = reflect.ValueOf(value)
			resultValue := fun.Func.Call(params)[0].Interface().(HandlerFunc)
			if resultValue.Method != "" && resultValue.Path != "" && resultValue.Func != nil {
				results = append(results, resultValue)
			}
		}
	}
	return results
}
func (obj *Client) Add(group *gin.RouterGroup, value any) {
	for _, resultValue := range obj.getHandlers(value) {
		if group == nil {
			obj.Handle(resultValue.Method, resultValue.Path, resultValue.Func)
		} else {
			group.Handle(resultValue.Method, resultValue.Path, resultValue.Func)
		}
	}
}
func (obj *Client) Run(addr ...string) error {
	return obj.enGine.Run(addr...)
}
func (obj *Client) Handle(httpMethod string, relativePath string, handlers ...gin.HandlerFunc) gin.IRoutes {
	return obj.enGine.Handle(httpMethod, relativePath, handlers...)
}
func (obj *Client) Group(relativePath string, handlers ...gin.HandlerFunc) *gin.RouterGroup { //返回分组
	return obj.enGine.Group(relativePath, handlers...)
}

func NewClient() *Client {
	return &Client{enGine: gin.Default()}
}
