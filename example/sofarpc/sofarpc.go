package main

import (
	"github.com/deepflowio/deepflow-wasm-go-sdk/sdk"
	_ "github.com/wasilibs/nottinygc"
)

func main() {
	sdk.Info("sofarpc-plugin loaded")
	parser := SofaRPCParser{}
	parser.Parser = interface{}(parser).(sdk.Parser)
	sdk.SetParser(parser)
}

type SofaRPCParser struct {
	sdk.DefaultParser
}

func (p SofaRPCParser) HookIn() []sdk.HookBitmap {
	return []sdk.HookBitmap{
		sdk.HOOK_POINT_CUSTOM_MESSAGE,
	}
}

func (p SofaRPCParser) CustomMessageHookIn() uint64 {
	return sdk.CustomMessageHookProtocol(sdk.PROTOCOL_SOFARPC, true)
}

func (p SofaRPCParser) OnCustomReq(ctx *sdk.CustomMessageCtx) sdk.Action {
	payload, err := ctx.BaseCtx.GetPayload()
	if err != nil {
		return sdk.ActionAbortWithErr(err)
	}
	reqLen := len(payload)
	sdk.Info("sofarpc req req_len=%d resp_len=%d payload_len=%d", reqLen, 0, reqLen)
	return sdk.CustomMessageActionAbortWithResult([]sdk.KeyVal{
		{
			Key: "sofarpc_example",
			Val: "request",
		},
		{
			Key: "sofarpc_req_attr",
			Val: "preset_req_value",
		},
	})
}

func (p SofaRPCParser) OnCustomResp(ctx *sdk.CustomMessageCtx) sdk.Action {
	payload, err := ctx.BaseCtx.GetPayload()
	if err != nil {
		return sdk.ActionAbortWithErr(err)
	}
	respLen := len(payload)
	sdk.Info("sofarpc resp req_len=%d resp_len=%d payload_len=%d", 0, respLen, respLen)
	return sdk.CustomMessageActionAbortWithResult([]sdk.KeyVal{
		{
			Key: "sofarpc_example",
			Val: "response",
		},
		{
			Key: "sofarpc_resp_attr",
			Val: "preset_resp_value",
		},
	})
}

func (p SofaRPCParser) OnCustomMessage(ctx *sdk.CustomMessageCtx) sdk.Action {
	baseCtx := &ctx.BaseCtx
	if baseCtx.Direction == sdk.DirectionRequest {
		return p.OnCustomReq(ctx)
	} else {
		return p.OnCustomResp(ctx)
	}
}
