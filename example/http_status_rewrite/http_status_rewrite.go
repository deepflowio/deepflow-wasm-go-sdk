/*
 * Copyright (c) 2022 Yunshan Networks
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
	"bufio"
	"bytes"
	"compress/gzip"
	"github.com/deepflowio/deepflow-wasm-go-sdk/sdk"
	"github.com/valyala/fastjson"
	"io"
	"net/http"
	"strings"
)

const WASM_HTTP_STATUS_REWRITE_PROTOCOL uint8 = 1

func main() {
	sdk.Info("on httpstatus rewrite wasm plugin init")
	sdk.SetParser(parser{})
}

type parser struct {
}

func (p parser) OnHttpReq(ctx *sdk.HttpReqCtx) sdk.HttpAction {
	return sdk.ActionNext()
}

func (p parser) OnHttpResp(ctx *sdk.HttpRespCtx) sdk.HttpAction {
	return sdk.ActionNext()
}

func (p parser) OnCheckPayload(ctx *sdk.ParseCtx) (protoNum uint8, protoStr string) {
	b, err := ctx.GetPayload()
	if err != nil {
		return 0, ""
	}
	r, _ := http.ReadRequest(bufio.NewReader(bytes.NewReader(b)))
	if r != nil {
		return WASM_HTTP_STATUS_REWRITE_PROTOCOL, "HTTP_CUSTOM"
	}
	return 0, ""
}

const BODY_START = `{"OPT_STATUS": "`

func (p parser) OnParsePayload(ctx *sdk.ParseCtx) sdk.ParseAction {
	if ctx.L7 != WASM_HTTP_STATUS_REWRITE_PROTOCOL {
		return sdk.ActionNext()
	}
	b, err := ctx.GetPayload()
	if err != nil {
		return sdk.ActionNext()
	}

	switch ctx.Direction {
	case sdk.DirectionRequest:
		r, _ := http.ReadRequest(bufio.NewReader(bytes.NewReader(b)))
		if r == nil {
			return sdk.ActionAbort()
		}
		return onReq(r)

	case sdk.DirectionResponse:
		r, _ := http.ReadResponse(bufio.NewReader(bytes.NewReader(b)), nil)
		if r == nil {
			return sdk.ActionAbort()
		}
		return onResp(r)
	default:
		return sdk.ActionNext()
	}

}

func (p parser) HookIn() []sdk.HookBitmap {
	return []sdk.HookBitmap{
		sdk.HOOK_POINT_PAYLOAD_PARSE,
	}
}

func onReq(req *http.Request) sdk.ParseAction {
	return sdk.ParseActionAbortWithL7Info([]*sdk.L7ProtocolInfo{
		{
			Req: &sdk.Request{
				ReqType:  req.Method,
				Domain:   req.Host,
				Resource: req.URL.Path,
				Endpoint: req.Host,
			},
		},
	})
}

/*
this demo use for convert and rewrite the response code according to the http response data in deepflow server.
deepflow server use the json key "OPT_STATUS" indicate the response status, "OPT_STATUS": "SUCCESS" is success,
otherwise assume fail and set the http status code to 500, the field map to deepflow as follows:

	response_code   -> http status code
	response_result -> if "OPT_STATUS": "SUCCESS" will leave it empty, otherwise will set to the whole http response body
	response_status -> http code in [200, 400) will act as Ok, [400, 500) will act as client error, [500,-) will act as server error
*/
func onResp(r *http.Response) sdk.ParseAction {
	var getStatus = func(statusCode int32) sdk.RespStatus {
		if statusCode >= 200 && statusCode < 400 {
			return sdk.RespStatusOk
		}
		if statusCode >= 400 && statusCode < 500 {
			return sdk.RespStatusClientErr
		}
		return sdk.RespStatusServerErr
	}

	var normalResp = func() sdk.ParseAction {
		code := int32(r.StatusCode)
		return sdk.ParseActionAbortWithL7Info([]*sdk.L7ProtocolInfo{
			{
				Resp: &sdk.Response{
					Status: getStatus(code),
					Code:   &code,
				},
			},
		})
	}

	var (
		buf  []byte
		body []byte
	)
	switch r.Header.Get("Content-Encoding") {
	case "gzip":
		g, err := gzip.NewReader(r.Body)
		if err != nil {
			sdk.Warn("%v", err)
			return normalResp()
		}

		body, _ = io.ReadAll(g)
		g.Close()
	default:
		body, _ = io.ReadAll(r.Body)
	}

	if len(body) == 0 {
		return normalResp()
	}

	/*
		due to tcp fragment, it is possible to receive the incomplete json data. if can not get the json key,
		try to get the OPT_STATUS from json start like `{"OPT_STATUS": "SOME STATUS"`, parse the key as much as possible.
		FIXME: remove the incomplete json data parse after agent implement tcp reassemble.
	*/
	status := fastjson.GetString(body, "OPT_STATUS")
	if status == "" {
		if !strings.HasPrefix(string(buf), BODY_START) {
			return normalResp()
		}
		buf = body[len(BODY_START):]

		for i := 0; i < len(buf); i++ {
			if buf[i] == '"' {
				status = string(buf[:i])
				break
			}
		}
	}
	attr := []sdk.KeyVal{
		{
			Key: "op_stat",
			Val: status,
		},
	}

	var (
		code   = int32(r.StatusCode)
		result string
	)
	switch status {
	case "SUCCESS":

	default:
		if code >= 200 && code < 300 {
			code = 500
			result = string(body)
		}
	}

	return sdk.ParseActionAbortWithL7Info([]*sdk.L7ProtocolInfo{
		{
			Resp: &sdk.Response{
				Status: getStatus(code),
				Code:   &code,
				Result: result,
			},
			Kv: attr,
		},
	})
}
