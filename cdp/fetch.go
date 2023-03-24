package cdp

import (
	"context"

	"gitee.com/baixudong/gospider/tools"
)

func (obj *WebSock) FetchEnable(preCtx context.Context) (RecvData, error) {
	return obj.send(preCtx, commend{
		Method: "Fetch.enable",
	})
}
func (obj *WebSock) FetchDisable(preCtx context.Context) (RecvData, error) {
	return obj.send(preCtx, commend{
		Method: "Fetch.disable",
	})
}
func (obj *WebSock) FetchContinueRequest(preCtx context.Context, requestId string) (RecvData, error) {
	return obj.send(preCtx, commend{
		Method: "Fetch.continueRequest",
		Params: map[string]any{
			"requestId": requestId,
		},
	})
}
func (obj *WebSock) FetchFailRequest(preCtx context.Context, requestId, errorReason string) (RecvData, error) {
	return obj.send(preCtx, commend{
		Method: "Fetch.failRequest",
		Params: map[string]any{
			"requestId":   requestId,
			"errorReason": errorReason,
		},
	})
}
func (obj *WebSock) FetchFulfillRequest(preCtx context.Context, requestId string, fulData FulData) (RecvData, error) {
	if fulData.Headers == nil {
		fulData.Headers = []Header{
			{
				Name:  "Content-Type",
				Value: tools.GetContentTypeWithBytes(tools.StringToBytes(fulData.Body)),
			},
		}
	}
	if fulData.StatusCode == 0 {
		fulData.StatusCode = 200
	}
	if fulData.ResponsePhrase == "" {
		fulData.ResponsePhrase = "200 OK"
	}

	return obj.send(preCtx, commend{
		Method: "Fetch.fulfillRequest",
		Params: map[string]any{
			"requestId":       requestId,
			"responseCode":    fulData.StatusCode,
			"responseHeaders": fulData.Headers,
			"body":            tools.Base64Encode(fulData.Body),
			"responsePhrase":  fulData.ResponsePhrase,
		},
	})
}
