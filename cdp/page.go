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

func (obj *WebSock) PageCaptureScreenshot(ctx context.Context, option LayoutMetrics) (RecvData, error) {
	return obj.send(ctx, commend{
		Method: "Page.captureScreenshot",
		Params: map[string]any{
			"format":                "png",
			"captureBeyondViewport": true,
			"clip": map[string]float64{
				"x":      option.CssContentSize.X,
				"y":      option.CssContentSize.Y,
				"width":  option.CssContentSize.Width,
				"height": option.CssContentSize.Height,
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
