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

import "encoding/binary"

//export on_http_req
func onHttpReq() bool {
	if vmParser == nil {
		return false
	}
	paramBuf := [PARSE_PARAM_BUF_SIZE]byte{}
	ctxSize := vmReadCtxBase(&paramBuf[0], len(paramBuf))
	if ctxSize == 0 {
		return false
	}
	reqInfo := [HTTP_REQ_BUF_SIZE]byte{}
	reqCtxSize := vmReadHttpReqInfo(&reqInfo[0], len(reqInfo))
	if reqCtxSize == 0 {
		return false
	}
	ctx := deserializeHttpReqCtx(paramBuf[:ctxSize], reqInfo[:reqCtxSize])
	if ctx == nil {
		return false
	}
	act := vmParser.OnHttpReq(ctx)
	if act == nil {
		return false
	}

	info, err := act.getParsePayloadResult()
	if err != nil {
		Error("on http req encounter error: %v", err)
		return act.abort()
	}
	if len(info) > 1 {
		Error("on http req return multi info")
		return act.abort()
	}

	data := serializeL7ProtocolInfo(info, DirectionRequest)
	if len(data) == 0 {
		return act.abort()
	}
	hostReadL7ProtocolInfo(&data[0], len(data))
	return act.abort()
}

//export on_http_resp
func onHttpResp() bool {
	if vmParser == nil {
		return false
	}
	paramBuf := [PARSE_PARAM_BUF_SIZE]byte{}
	ctxSize := vmReadCtxBase(&paramBuf[0], len(paramBuf))
	if ctxSize == 0 {
		return false
	}
	respInfo := [HTTP_RESP_BUF_SIZE]byte{}
	respCtxSize := vmReadHttpRespInfo(&respInfo[0], len(respInfo))
	if respCtxSize == 0 {
		return false
	}
	ctx := deserializeHttpRespCtx(paramBuf[:ctxSize], respInfo[:respCtxSize])
	if ctx == nil {
		return false
	}
	act := vmParser.OnHttpResp(ctx)

	if act == nil {
		return false
	}

	info, err := act.getParsePayloadResult()
	if err != nil {
		Error("on http req encounter error: %v", err)
		return act.abort()
	}

	if len(info) > 1 {
		Error("on http resp return multi info")
		return act.abort()
	}
	if len(info) > 0 && info[0].Resp.Status == nil {
		// preserve the status if not rewrite
		info[0].Resp.Status = &ctx.Status
	}

	data := serializeL7ProtocolInfo(info, DirectionResponse)
	if len(data) == 0 {
		return act.abort()
	}
	hostReadL7ProtocolInfo(&data[0], len(data))
	return act.abort()
}

//export on_custom_message
func onCustomMessage() bool {
	if vmParser == nil {
		return false
	}
	paramBuf := [PARSE_PARAM_BUF_SIZE]byte{}
	ctxSize := vmReadCtxBase(&paramBuf[0], len(paramBuf))
	if ctxSize == 0 {
		return false
	}
	customMessageInfo := [CUSTOM_MESSAGE_BUF_SIZE]byte{}
	messageCtxSize := vmReadCustomMessageInfo(&customMessageInfo[0], len(customMessageInfo))
	if messageCtxSize == 0 {
		return false
	}
	ctx := deserializeCustomMessageCtx(paramBuf[:ctxSize], customMessageInfo[:messageCtxSize])
	if ctx == nil {
		return false
	}
	act := vmParser.OnCustomMessage(ctx)
	if act == nil {
		return false
	}

	info, err := act.getParsePayloadResult()
	if err != nil {
		Error("on custom message encounter error: %v", err)
		return act.abort()
	}
	if len(info) > 1 {
		Error("on custom message return multi info")
		return act.abort()
	}

	data := serializeL7ProtocolInfo(info, ctx.BaseCtx.Direction)
	if len(data) == 0 {
		return act.abort()
	}
	hostReadL7ProtocolInfo(&data[0], len(data))
	return act.abort()
}

//export check_payload
func checkPayload() uint8 {
	if vmParser == nil {
		return 0
	}
	paramBuf := [PARSE_PARAM_BUF_SIZE]byte{}
	ctxSize := vmReadCtxBase(&paramBuf[0], len(paramBuf))
	if ctxSize == 0 {
		return 0
	}
	parseCtx := deserializeParseCtx(paramBuf[:ctxSize])
	if parseCtx == nil {
		return 0
	}
	protoNum, protoStr := vmParser.OnCheckPayload(parseCtx)
	if len(protoStr) > 16 {
		protoStr = protoStr[:16]
	}

	buf := make([]byte, len(protoStr)+2)
	off := 0
	writeStr(protoStr, buf, &off)

	if protoNum == 0 || !hostReadStrResult(&buf[0], len(buf)) {
		return 0
	}

	return protoNum
}

//export parse_payload
func parsePayload() bool {
	if vmParser == nil {
		return false
	}
	paramBuf := [PARSE_PARAM_BUF_SIZE]byte{}
	ctxSize := vmReadCtxBase(&paramBuf[0], len(paramBuf))
	if ctxSize == 0 {
		return false
	}
	parseCtx := deserializeParseCtx(paramBuf[:ctxSize])
	if parseCtx == nil {
		return false
	}
	act := vmParser.OnParsePayload(parseCtx)
	if act == nil {
		return false
	}

	infos, err := act.getParsePayloadResult()

	if err != nil {
		Error("on parse payload encounter error: %v", err)
		return act.abort()
	}

	// when abort with no error, set the result and write to host
	if len(infos) == 0 {
		return act.abort()
	}

	data := serializeL7ProtocolInfo(infos, parseCtx.Direction)
	if len(data) == 0 {
		return act.abort()
	}
	hostReadL7ProtocolInfo(&data[0], len(data))
	return act.abort()
}

//export get_hook_bitmap
func getHookBitmap() *byte {
	if vmParser == nil {
		return nil
	}
	b := vmParser.HookIn()
	hookBit := [2]uint64{0, 0}

	for _, v := range b {
		hookBit[0] |= v[0]
		hookBit[1] |= v[1]
	}

	bitmap := [16]byte{}
	binary.BigEndian.PutUint64(bitmap[:8], hookBit[0])
	binary.BigEndian.PutUint64(bitmap[8:], hookBit[1])
	return &bitmap[0]
}

//export get_custom_message_hook
func getCustomMessageHook() *byte {
	if vmParser == nil {
		return nil
	}
	data := [8]byte{}
	binary.BigEndian.PutUint64(data[:], vmParser.CustomMessageHookIn())
	return &data[0]
}
