/*
 * Copyright (c) 2024 Yunshan Networks
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"github.com/deepflowio/deepflow-wasm-go-sdk/sdk"
	"github.com/valyala/fastjson"
	_ "github.com/wasilibs/nottinygc"
)

func main() {
	sdk.Info("dubbo-plugin loaded")
	parser := DubboParser{}
	parser.Parser = interface{}(parser).(sdk.Parser)
	sdk.SetParser(parser)
}

type DubboParser struct {
	sdk.DefaultParser
}

func (p DubboParser) HookIn() []sdk.HookBitmap {
	return []sdk.HookBitmap{
		sdk.HOOK_POINT_CUSTOM_MESSAGE,
	}
}

func (p DubboParser) CustomMessageHookIn() uint64 {
	return sdk.CustomMessageHookProtocol(sdk.PROTOCOL_DUBBO, true)
}

func (p DubboParser) OnCustomReq(ctx *sdk.CustomMessageCtx) sdk.Action {
	baseCtx := &ctx.BaseCtx
	payload, err := baseCtx.GetPayload() // The content of the Dubbo protocol, for reference: https://cn.dubbo.apache.org/zh/blog/2018/10/05/dubbo-%e5%8d%8f%e8%ae%ae%e8%af%a6%e8%a7%a3/#%E5%8D%8F%E8%AE%AE%E6%A6%82%E8%A7%88
	if err != nil {
		return sdk.ActionAbortWithErr(err)
	}
	_ = payload
	// Do something, such as parsing the payload to obtain Dubbo's Dubbo version, Service name, Service version, Method name, and so on.
	// return sdk.ParseActionAbortWithL7Info([]*sdk.L7ProtocolInfo{{

	// 	Req: &sdk.Request{
	// 		Domain:  "custom_service_name", // Rewrite service_name
	// 		ReqType: "custom_req_type",     // Rewrite req_type
	// 	},
	// 	Kv: []sdk.KeyVal{{
	// 		Key: "custom_extra_info_key", // Add extra information
	// 		Val: "custom_extra_info_value",
	// 	}},
	// }})
	return sdk.ActionAbort()
}

func (p DubboParser) OnCustomResp(ctx *sdk.CustomMessageCtx) sdk.Action {
	baseCtx := &ctx.BaseCtx
	payload, err := baseCtx.GetPayload() // The content of the Dubbo protocol, for reference: https://cn.dubbo.apache.org/zh/blog/2018/10/05/dubbo-%e5%8d%8f%e8%ae%ae%e8%af%a6%e8%a7%a3/#%E5%8D%8F%E8%AE%AE%E6%A6%82%E8%A7%88
	if err != nil {
		return sdk.ActionAbortWithErr(err)
	}
	if len(payload) < 17 { // dubbo headers(16bytes) + return value's type (1byte)
		sdk.Warn("empty return value")
		return sdk.ActionAbort()
	}
	dubboReturnValue := payload[17:]
	status := sdk.RespStatusOk
	status_code := int32(0)
	exception := ""
	// Assuming the returned value bytes are data serialized in JSON format, the content is: {"status code": 500, "exception": "internal error"}
	// Extract the status_code and exception from the data
	if fastjson.Exists(dubboReturnValue, "status_code") {
		code := fastjson.GetInt(dubboReturnValue, "status_code")
		status_code = int32(code)
		if status_code >= 500 {
			status = sdk.RespStatusServerErr
		} else if status_code >= 400 && status_code < 500 {
			status = sdk.RespStatusClientErr
		}
	}
	if fastjson.Exists(dubboReturnValue, "exception") {
		exception = fastjson.GetString(dubboReturnValue, "exception")
	}

	return sdk.ParseActionAbortWithL7Info([]*sdk.L7ProtocolInfo{
		{
			Resp: &sdk.Response{
				Code:      &status_code,             // Overwrite the status code in the response message, for example, if the original Dubbo status code is 20, override it with a custom status code
				Status:    &status,                  // Rewrite the response status to a custom status
				Result:    string(dubboReturnValue), // The entire payload can be placed into Response.Result for easier troubleshooting
				Exception: exception,                // Rewrite exception
			},
			Kv: []sdk.KeyVal{{
				Key: "custom_exception", // Add extra information
				Val: exception,
			}},
		},
	})
}

func (p DubboParser) OnCustomMessage(ctx *sdk.CustomMessageCtx) sdk.Action {
	baseCtx := &ctx.BaseCtx
	if baseCtx.Direction == sdk.DirectionRequest {
		return p.OnCustomReq(ctx)
	} else {
		return p.OnCustomResp(ctx)
	}
}
