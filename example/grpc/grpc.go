package main

import (
	"github.com/deepflowio/deepflow-wasm-go-sdk/sdk"
	sdkpb "github.com/deepflowio/deepflow-wasm-go-sdk/sdk/pb"
)

//go:generate mkdir -p pb
//go:generate bash -c "cd protoc-gen-demo && go build"
//go:generate protoc --go_out=./pb --demo_out=./pb --plugin=protoc-gen-demo=./protoc-gen-demo/protoc-gen-demo --go-vtproto_out=./pb --go-vtproto_opt=features=unmarshal ./demo.proto

func main() {
	sdk.SetParser(SomeParser{})
	sdk.Warn("plugin loaded")
}

type SomeParser struct {
}

func (p SomeParser) OnCustomMessage(ctx *sdk.CustomMessageCtx) sdk.Action {
	return sdk.ActionNext()
}

func (p SomeParser) OnNatsMessage(message sdkpb.NatsMessage) sdk.Action {
	return sdk.ActionNext()
}

func (p SomeParser) CustomMessageHookIn() uint64 {
	return 0
}

func (p SomeParser) HookIn() []sdk.HookBitmap {
	return []sdk.HookBitmap{
		// 这里的钩子是在 HTTP/HTTP2/gRPC 解析中，目的是增强 HTTP/HTTP2/gRPC 协议解析的能力，
		// 解析方法和内容由用户自定义，但是日志解析后的协议字段依旧为 HTTP/HTTP2/gRPC.
		sdk.HOOK_POINT_HTTP_REQ,
		sdk.HOOK_POINT_HTTP_RESP,
	}
}

func (p SomeParser) OnHttpReq(ctx *sdk.HttpReqCtx) sdk.Action {
	info := &sdk.L7ProtocolInfo{
		Req:           &sdk.Request{},
		ProtocolMerge: true,
		IsEnd:         true,
		Trace: &sdk.Trace{
			TraceID: "this-trace-id-from-wasm",
		},
	}
	return sdk.ParseActionAbortWithL7Info([]*sdk.L7ProtocolInfo{info})
}

func (p SomeParser) OnHttpResp(ctx *sdk.HttpRespCtx) sdk.Action {
	return sdk.ActionNext()
}

func (p SomeParser) OnCheckPayload(ctx *sdk.ParseCtx) (uint8, string) {
	// 这里是协议判断的逻辑， 返回 0 表示失败
	return 1, "GrpcTraceID"
}

func (p SomeParser) OnParsePayload(ctx *sdk.ParseCtx) sdk.Action {
	// 这里是解析协议的逻辑
	if ctx.L4 != sdk.TCP || ctx.L7 != 1 {
		return sdk.ActionNext()
	}
	return sdk.ActionNext()
}
