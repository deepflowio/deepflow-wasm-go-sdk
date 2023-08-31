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
	"net"

	"golang.org/x/net/dns/dnsmessage"

	"github.com/deepflowio/deepflow-wasm-go-sdk/sdk"
)

const WASM_DNS_PROTOCOL uint8 = 1

type dnsParser struct{}

func (p dnsParser) HookIn() []sdk.HookBitmap {
	return []sdk.HookBitmap{
		sdk.HOOK_POINT_PAYLOAD_PARSE,
	}
}

func (p dnsParser) OnHttpReq(ctx *sdk.HttpReqCtx) sdk.Action {
	return sdk.ActionNext()
}

func (p dnsParser) OnHttpResp(ctx *sdk.HttpRespCtx) sdk.Action {
	return sdk.ActionNext()
}

// check whether is request, return 0 indicate check fail, other value indicate is the dns request.
// agent will use it to determind the direction, must return 0 if is response
func (p dnsParser) OnCheckPayload(ctx *sdk.ParseCtx) (uint8, string) {
	if ctx.L4 != sdk.UDP || ctx.DstPort != 53 {
		return 0, ""
	}

	payload, err := ctx.GetPayload()
	if err != nil {
		sdk.Error("get payload fail: %v", err)
		return 0, ""
	}
	var dns dnsmessage.Message
	if err := dns.Unpack(payload); err != nil {
		return 0, ""
	}
	if dns.Response {
		return 0, ""
	}
	return WASM_DNS_PROTOCOL, "dns"
}

func (p dnsParser) OnParsePayload(ctx *sdk.ParseCtx) sdk.Action {
	if ctx.L4 != sdk.UDP || ctx.L7 != WASM_DNS_PROTOCOL {
		return sdk.ActionNext()
	}

	payload, err := ctx.GetPayload()
	if err != nil {
		return sdk.ActionAbortWithErr(err)
	}

	var dns dnsmessage.Message
	if err := dns.Unpack(payload); err != nil {
		return sdk.ActionAbortWithErr(err)
	}
	var (
		req  *sdk.Request
		resp *sdk.Response
		id   = uint32(dns.ID)
	)
	switch ctx.Direction {
	case sdk.DirectionRequest:
		for _, v := range dns.Questions {
			if v.Type == dnsmessage.TypeA || v.Type == dnsmessage.TypeAAAA {
				req = &sdk.Request{
					ReqType:  v.Type.String(),
					Resource: v.Name.String(),
				}
				break
			}
		}
		if req == nil {
			sdk.Warn("%s:%d -> %s:%d dns question no A or AAAA record ", ctx.SrcIP.IP, ctx.SrcPort, ctx.DstIP.IP, ctx.DstPort)
			return sdk.ActionAbort()
		}
	case sdk.DirectionResponse:
		for _, v := range dns.Answers {
			if v.Header.Type == dnsmessage.TypeA || v.Header.Type == dnsmessage.TypeAAAA {
				var ip net.IP
				switch r := v.Body.(type) {
				case *dnsmessage.AAAAResource:
					ip = r.AAAA[:]
				case *dnsmessage.AResource:
					ip = r.A[:]
				}
				status := sdk.RespStatusOk
				resp = &sdk.Response{
					Status: &status,
					Result: ip.String(),
				}
				break
			}
		}
		if resp == nil {
			sdk.Warn("%s:%d -> %s:%d dns response no A or AAAA record ", ctx.SrcIP.IP, ctx.SrcPort, ctx.DstIP.IP, ctx.DstPort)
			return sdk.ActionAbort()
		}
	default:
		panic("unreachable")
	}

	return sdk.ParseActionAbortWithL7Info([]*sdk.L7ProtocolInfo{
		{
			RequestID: &id,
			Req:       req,
			Resp:      resp,
		},
	})
}

func main() {
	sdk.Warn("wasm register dns parser")
	sdk.SetParser(dnsParser{})
}
