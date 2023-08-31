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
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deepflowio/deepflow-wasm-go-sdk/sdk"
	"github.com/valyala/fastjson"
)

type httpHook struct {
}

func (p httpHook) HookIn() []sdk.HookBitmap {
	return []sdk.HookBitmap{
		sdk.HOOK_POINT_HTTP_REQ,
		sdk.HOOK_POINT_HTTP_RESP,
	}
}

/*
assume the http request as follow:

	GET /user_info?username=test&type=1 HTTP/1.1
	Custom-Trace-Info: trace_id: xxx, span_id: sss
*/
func (p httpHook) OnHttpReq(ctx *sdk.HttpReqCtx) sdk.Action {
	baseCtx := &ctx.BaseCtx
	if baseCtx.DstPort != 8080 || !strings.HasPrefix(ctx.Path, "/user_info?") {
		return sdk.ActionNext()
	}

	payload, err := baseCtx.GetPayload()
	if err != nil {
		return sdk.ActionAbortWithErr(err)
	}

	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(payload)))
	if err != nil {
		return sdk.ActionAbortWithErr(err)
	}

	query := req.URL.Query()

	attr := []sdk.KeyVal{
		{
			Key: "username",
			Val: query.Get("username"),
		},

		{
			Key: "type",
			Val: query.Get("type"),
		},
	}

	var (
		traceID string
		spanID  string
		trace   *sdk.Trace
	)

	traceInfo, ok := req.Header["Custom-Trace-Info"]
	if ok && len(traceInfo) != 0 {
		s := strings.Split(traceInfo[0], ",")
		if len(s) == 2 {
			t := strings.Split(s[0], ":")
			if len(t) == 2 {
				traceID = strings.TrimSpace(t[1])
			}

			sp := strings.Split(s[1], ":")
			if len(sp) == 2 {
				spanID = strings.TrimSpace(sp[1])
			}
		}

	}

	if traceID != "" && spanID != "" {
		trace = &sdk.Trace{
			TraceID: traceID,
			SpanID:  spanID,
		}
	}

	return sdk.HttpReqActionAbortWithResult(nil, trace, attr)
}

/*
assume resp as follow:

	HTTP/1.1 200 OK

	{"code": 0, "data": {"user_id": 12345, "register_time": 1682050409}}
*/
func (p httpHook) OnHttpResp(ctx *sdk.HttpRespCtx) sdk.Action {
	baseCtx := &ctx.BaseCtx
	if baseCtx.SrcPort != 8080 {
		return sdk.ActionNext()
	}
	payload, err := baseCtx.GetPayload()
	if err != nil {
		return sdk.ActionAbortWithErr(err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(payload)), nil)
	if err != nil {
		return sdk.ActionAbortWithErr(err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return sdk.ActionAbortWithErr(err)
	}
	if fastjson.Exists(body, "code") && fastjson.Exists(body, "data") {
		code := fastjson.GetInt(body, "code")
		if code == 0 {
			userID := fastjson.GetInt(body, "data", "user_id")
			t := fastjson.GetInt(body, "data", "register_time")

			return sdk.HttpRespActionAbortWithResult(nil, nil, []sdk.KeyVal{
				{
					Key: "user_id",
					Val: strconv.Itoa(userID),
				},

				{
					Key: "register_time",
					Val: time.Unix(int64(t), 0).String(),
				},
			})
		}

	}
	return sdk.ActionAbort()

}

func (p httpHook) OnCheckPayload(baseCtx *sdk.ParseCtx) (uint8, string) {
	return 0, ""
}

func (p httpHook) OnParsePayload(baseCtx *sdk.ParseCtx) sdk.Action {
	return sdk.ActionNext()
}

func main() {
	sdk.Warn("wasm register http hook")
	sdk.SetParser(httpHook{})

}
