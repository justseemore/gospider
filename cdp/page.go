package cdp

import (
	"context"
)

func (obj *WebSock) PageEnable(ctx context.Context) (RecvData, error) {
	return obj.send(ctx, commend{
		Method: "Page.enable",
	})
}
func (obj *WebSock) PageAddScriptToEvaluateOnNewDocument(ctx context.Context, source string) (RecvData, error) {
	return obj.send(ctx, commend{
		Method: "Page.addScriptToEvaluateOnNewDocument",
		Params: map[string]any{
			"source": source,
		},
	})
}

type ScreenShotOption struct {
	Format                string //图像压缩格式（默认为 webp）,允许的值：jpeg,png,webp
	Quality               int    //范围 [0..100] 的压缩质量（仅限 jpeg）。
	CaptureBeyondViewport bool   //捕获视口之外的屏幕截图。默认为 false。
}

func (obj *WebSock) PageCaptureScreenshot(ctx context.Context, rect Rect, options ...ScreenShotOption) (RecvData, error) {
	var option ScreenShotOption
	if len(options) > 0 {
		option = options[0]
	}
	if option.Format == "" {
		option.Format = "webp"
	}
	return obj.send(ctx, commend{
		Method: "Page.captureScreenshot",
		Params: map[string]any{
			"format":                option.Format,
			"quality":               option.Quality,
			"captureBeyondViewport": option.CaptureBeyondViewport,
			"optimizeForSpeed":      true,
			"clip": map[string]float64{
				"x":      rect.X,
				"y":      rect.Y,
				"width":  rect.Width,
				"height": rect.Height,
				"scale":  1,
			},
		},
	})
}
func (obj *WebSock) PageGetLayoutMetrics(ctx context.Context) (RecvData, error) {
	return obj.send(ctx, commend{
		Method: "Page.getLayoutMetrics",
		Params: map[string]any{},
	})
}
func (obj *WebSock) PageReload(ctx context.Context) (RecvData, error) {
	return obj.send(ctx, commend{
		Method: "Page.reload",
		Params: map[string]any{},
	})
}
func (obj *WebSock) PageNavigate(ctx context.Context, url string) (RecvData, error) {
	return obj.send(ctx, commend{
		Method: "Page.navigate",
		Params: map[string]any{
			"url":    url,
			"width":  1080,
			"height": 720,
		},
	})
}
