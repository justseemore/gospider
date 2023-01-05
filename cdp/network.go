package cdp

import "context"

type Cookie struct {
	Name         string  `json:"name,omitempty"`  //必填
	Value        string  `json:"value,omitempty"` //必填
	Url          string  `json:"url,omitempty"`
	Domain       string  `json:"domain,omitempty"` //必填
	Path         string  `json:"path,omitempty"`
	Secure       bool    `json:"secure,omitempty"`
	HttpOnly     bool    `json:"httpOnly,omitempty"`
	SameSite     string  `json:"sameSite,omitempty"`
	Expires      float64 `json:"expires,omitempty"`
	Priority     string  `json:"priority,omitempty"`
	SameParty    bool    `json:"sameParty,omitempty"`
	SourceScheme string  `json:"sourceScheme,omitempty"`
	SourcePort   int     `json:"sourcePort,omitempty"`
	PartitionKey int     `json:"partitionKey,omitempty"`
	Session      bool    `json:"session,omitempty"`
	Size         int64   `json:"size,omitempty"`
}

func (obj *WebSock) NetworkSetCookies(preCtx context.Context, cookies []Cookie) (RecvData, error) {
	return obj.send(preCtx, commend{
		Method: "Network.setCookies",
		Params: map[string]any{
			"cookies": cookies,
		},
	})
}
func (obj *WebSock) NetworkGetCookies(preCtx context.Context, urls ...string) (RecvData, error) {
	return obj.send(preCtx, commend{
		Method: "Network.getCookies",
		Params: map[string]any{
			"urls": urls,
		},
	})
}
func (obj *WebSock) NetworkSetCacheDisabled(preCtx context.Context, cacheDisabled bool) (RecvData, error) {
	return obj.send(preCtx, commend{
		Method: "Network.setCacheDisabled",
		Params: map[string]any{
			"cacheDisabled": cacheDisabled,
		},
	})
}
