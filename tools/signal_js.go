//go:build js

package tools

import (
	"context"
)

func Signal(preCtx context.Context, fun func()) {
	if fun != nil {
		select {
		case <-preCtx.Done():
			fun()
		case s := <-ch:
			fun()
		}
	}
}
