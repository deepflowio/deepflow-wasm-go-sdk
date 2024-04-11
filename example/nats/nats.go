package main

import (
	"strings"

	"github.com/deepflowio/deepflow-wasm-go-sdk/example/nats/pb"
	"github.com/deepflowio/deepflow-wasm-go-sdk/sdk"
	sdkpb "github.com/deepflowio/deepflow-wasm-go-sdk/sdk/pb"
)

var kv map[string]string

//go:generate mkdir -p pb
//go:generate bash -c "cd protoc-gen-demo && go build"
//go:generate protoc --go_out=./pb --demo_out=./pb --plugin=protoc-gen-demo=./protoc-gen-demo/protoc-gen-demo --go-vtproto_out=./pb --go-vtproto_opt=features=unmarshal ./demo.proto
func main() {
	sdk.Info("nrpc-parser loaded")
	parser := NrpcParser{}
	parser.Parser = interface{}(parser).(sdk.Parser)
	sdk.SetParser(parser)
	kv = make(map[string]string)
}

type NrpcParser struct {
	sdk.DefaultParser
}

func (p NrpcParser) HookIn() []sdk.HookBitmap {
	return []sdk.HookBitmap{
		sdk.HOOK_POINT_CUSTOM_MESSAGE,
	}
}

func (p NrpcParser) CustomMessageHookIn() uint64 {
	// return sdk.CUSTOM_MESSAGE_HOOK_ALL
	return sdk.CustomMessageHookProtocol(sdk.PROTOCOL_NATS, true)
}

func (p NrpcParser) OnNatsMessage(message sdkpb.NatsMessage) sdk.Action {
	var service string
	var method string
	var isRequest bool
	var function string
	var callId string

	if len(message.ReplyTo) > 0 {
		function = message.Subject
		callId = message.ReplyTo
		kv[callId] = function
		isRequest = true
	} else {
		var ok bool
		function, ok = kv[message.Subject]
		callId = message.Subject
		if !ok {
			return sdk.ActionNext()
		}
		isRequest = false
	}

	if len(function) > 0 {
		pos := strings.Index(function, ".")
		if pos > 0 {
			service = function[:pos]
			method = function[pos+1:]
		} else {
			service = ""
			method = ""
		}
	}
	jsonStr := string(pb.ProtobufToJson(service, method, isRequest, []byte(message.Payload)))
	return sdk.ParseActionAbortWithL7Info([]*sdk.L7ProtocolInfo{{
		Resp:  &sdk.Response{},
		Req:   &sdk.Request{},
		Trace: nil,
		Kv: []sdk.KeyVal{
			{
				Key: "json_payload",
				Val: jsonStr,
			},
			{
				Key: "call_id",
				Val: callId,
			},
		},
		L7ProtocolStr: "nRPC",
	}})
}
