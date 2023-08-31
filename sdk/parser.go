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

package sdk

var (
	vmParser Parser
)

func SetParser(p Parser) {
	vmParser = p
}

// u128
type HookBitmap [2]uint64

/*
correspond agent const:

	type HookPiont = u128;
	pub(super) const HOOK_POINT_HTTP_REQ: HookPiont = 1 << 127;
	pub(super) const HOOK_POINT_HTTP_RESP: HookPiont = 1 << 126;

	pub(super) const HOOK_POINT_PAYLOAD_PARSE: u128 = 1;
*/
var (
	// correspond Parser.OnHttpReq
	HOOK_POINT_HTTP_REQ HookBitmap = [2]uint64{1 << 63, 0}
	// correspond Parser.OnHttpResp
	HOOK_POINT_HTTP_RESP HookBitmap = [2]uint64{1 << 62, 0}
	// correspond Parser.OnCheckPayload and Parser.OnParsePayload
	HOOK_POINT_PAYLOAD_PARSE HookBitmap = [2]uint64{0, 1}
)

type KeyVal struct {
	Key string
	Val string
}

type Action interface {
	abort() bool
	getParsePayloadResult() ([]*L7ProtocolInfo, error)
}

func ActionAbort() Action {
	return &action{
		isAbort: true,
	}
}

func ActionAbortWithErr(err error) Action {
	return &action{
		isAbort: true,
		e:       err,
	}
}

func ActionNext() Action {
	return &action{
		isAbort: false,
	}
}

// agent will traversal to run all plugins, abort will abort the traversal, abort with no error will write the result to host.
type Parser interface {
	OnHttpReq(*HttpReqCtx) Action
	OnHttpResp(*HttpRespCtx) Action
	// protoNum return 0 indicate fail
	OnCheckPayload(*ParseCtx) (protoNum uint8, protoStr string)
	OnParsePayload(*ParseCtx) Action
	HookIn() []HookBitmap
}

type action struct {
	e             error
	isAbort       bool
	payloadResult []*L7ProtocolInfo
}

func (a *action) getParsePayloadResult() ([]*L7ProtocolInfo, error) {
	return a.payloadResult, a.e
}

func (e *action) abort() bool {
	return e.isAbort
}

func HttpReqActionAbortWithResult(req *Request, trace *Trace, kv []KeyVal) Action {
	if req == nil {
		req = &Request{}
	}
	return &action{
		isAbort: true,
		payloadResult: []*L7ProtocolInfo{
			{
				Req:   req,
				Trace: trace,
				Kv:    kv,
			},
		},
	}
}

func HttpRespActionAbortWithResult(resp *Response, trace *Trace, kv []KeyVal) Action {
	if resp == nil {
		resp = &Response{}
	}
	return &action{
		isAbort: true,
		payloadResult: []*L7ProtocolInfo{
			{
				Resp:  resp,
				Trace: trace,
				Kv:    kv,
			},
		},
	}
}

func ParseActionAbortWithL7Info(info []*L7ProtocolInfo) Action {
	return &action{
		isAbort:       true,
		payloadResult: info,
	}
}
