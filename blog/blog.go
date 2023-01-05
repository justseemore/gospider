package blog

import (
	"fmt"
	"log"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type ClientOption struct {
	Fields map[string]any //添加到json日志中的内容
}
type Client struct {
	logger *zap.Logger
}

func map2field(maps []map[string]any) []zapcore.Field {
	datas := []zapcore.Field{}
	for _, lls := range maps {
		for key, val := range lls {
			datas = append(datas, zap.Any(key, val))
		}
	}
	return datas
}

// 创建日志客户端
func NewClient(fileName string, fields ...map[string]any) *Client {
	if fileName == "" {
		log.Print("文件名不能为空")
		return nil
	}
	var field map[string]any
	if len(fields) > 0 {
		field = fields[0]
	}
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:     "time",
		LevelKey:    "level",
		MessageKey:  "msg",
		EncodeLevel: zapcore.LowercaseLevelEncoder, // 小写编码器
		EncodeTime:  zapcore.ISO8601TimeEncoder,    // ISO8601 UTC 时间格式
	}
	config := zap.Config{
		Level:         zap.NewAtomicLevelAt(zap.DebugLevel), // 日志级别
		Development:   true,                                 // 开发模式，堆栈跟踪
		Encoding:      "json",                               // 输出格式 console 或 json
		EncoderConfig: encoderConfig,                        // 编码器配置
		InitialFields: field,                                // 初始化字段，如：添加一个服务器名称
	}
	config.OutputPaths = []string{fileName}
	config.ErrorOutputPaths = []string{fileName}
	logger, err := config.Build()
	if err != nil {
		log.Print(err)
		return nil
	}
	return &Client{logger: logger}
}
func (obj *Client) Debug(val string, fields ...map[string]any) {
	obj.logger.Debug(fmt.Sprint(val), map2field(fields)...)
}
func (obj *Client) Info(val any, fields ...map[string]any) {
	obj.logger.Info(fmt.Sprint(val), map2field(fields)...)
}
func (obj *Client) Warn(val any, fields ...map[string]any) {
	obj.logger.Warn(fmt.Sprint(val), map2field(fields)...)
}
func (obj *Client) Error(val any, fields ...map[string]any) {
	obj.logger.Error(fmt.Sprint(val), map2field(fields)...)
}
func (obj *Client) Panic(val any, fields ...map[string]any) {
	obj.logger.Panic(fmt.Sprint(val), map2field(fields)...)
}
func (obj *Client) Fatal(val any, fields ...map[string]any) {
	obj.logger.Fatal(fmt.Sprint(val), map2field(fields)...)
}

// 黑色:0,红色:1,绿色:2,黄色:3,蓝色:4,紫红色:5,青蓝色:6,白色:7
// fgColor:字体颜色,bgColor:背景颜色
func Color(fgColor, bgColor int, texts ...any) string {
	return fmt.Sprintf("%c[0;%d;%dm%s%c[0m", 0x1B, 40+bgColor, 30+fgColor, fmt.Sprint(texts...), 0x1B)
}
