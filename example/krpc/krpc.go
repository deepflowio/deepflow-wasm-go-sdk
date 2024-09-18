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
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"

	"github.com/deepflowio/deepflow-wasm-go-sdk/example/krpc/pb"
	"github.com/deepflowio/deepflow-wasm-go-sdk/sdk"
	_ "github.com/wasilibs/nottinygc"
)

const (
	KRPC_FIX_HDR_LEN int   = 8
	KRPC_DIR_REQ     int32 = 1
	KRPC_DIR_RESP    int32 = 2
	KRPC_PROTOCOL          = 1
)

var protocolErr = errors.New("unknown protocol")

type KrpcInfo struct {
	Rrt      uint64
	MsgType  sdk.Direction
	MsgId    int32
	ServId   int32
	Sequence int32
	// 0 success, negative indicate error, no positive number.
	RetCode int32

	// trace info
	TraceId      string
	SpanId       string
	ParentSpanId string
	Status       sdk.RespStatus
}

func (k *KrpcInfo) fillFromPb(meta *pb.KrpcMeta) error {
	switch int32(meta.Direction) {
	case KRPC_DIR_REQ:
		k.MsgType = sdk.DirectionRequest
	case KRPC_DIR_RESP:
		k.MsgType = sdk.DirectionResponse
	default:
		return errors.New("unknown krpc direction")
	}
	k.MsgId = meta.MsgId
	k.ServId = meta.ServiceId
	k.Sequence = meta.Sequence
	k.RetCode = meta.RetCode
	if meta.Trace != nil {
		k.TraceId = meta.Trace.TraceId
		k.SpanId = meta.Trace.SpanId
		k.ParentSpanId = meta.Trace.ParentSpanId
	}

	if k.RetCode == 0 {
		k.Status = sdk.RespStatusOk
	} else {
		k.Status = sdk.RespStatusServerErr
	}
	return nil
}

/*
krpc hdr reference https://github.com/bruceran/krpc/blob/master/doc/develop.md#krpc%E7%BD%91%E7%BB%9C%E5%8C%85%E5%8D%8F%E8%AE%AE

0  .......8........16........24.........32
1  |-----KR---------|----- headLen--------|
2  |---------------packetLen--------------|
*/
func (k *KrpcInfo) parse(payload []byte, check bool) error {
	if len(payload) < KRPC_FIX_HDR_LEN || !bytes.Equal(payload[:2], []byte("KR")) {
		return protocolErr
	}
	hdrLen := int(binary.BigEndian.Uint16(payload[2:]))

	var pbPayload []byte

	if hdrLen+KRPC_FIX_HDR_LEN > len(payload) {
		if check {
			return protocolErr
		}
		pbPayload = payload[KRPC_FIX_HDR_LEN:]
	} else {
		pbPayload = payload[KRPC_FIX_HDR_LEN : KRPC_FIX_HDR_LEN+hdrLen]
	}
	krpcPb := &pb.KrpcMeta{}
	if err := krpcPb.UnmarshalVT(pbPayload); err != nil && check {
		return protocolErr
	}
	err := k.fillFromPb(krpcPb)
	if err == nil {
		if k.isHeartBeat() || check {
			return nil
		}
	}
	return err
}

func (k *KrpcInfo) isHeartBeat() bool {
	// reference https://github.com/bruceran/krpc/blob/master/doc/develop.md#krpc%E7%BD%91%E7%BB%9C%E5%8C%85%E5%8D%8F%E8%AE%AE
	return k.Sequence == 0 && k.MsgId == 1 && k.ServId == 1
}

type parser struct {
	sdk.DefaultParser
}

func (p parser) OnHttpReq(ctx *sdk.HttpReqCtx) sdk.Action {
	return sdk.ActionNext()
}

func (p parser) OnHttpResp(ctx *sdk.HttpRespCtx) sdk.Action {
	return sdk.ActionNext()
}

func (p parser) OnCheckPayload(ctx *sdk.ParseCtx) (protoNum uint8, protoStr string) {
	payload, err := ctx.GetPayload()
	if err != nil || ctx.L4 != sdk.TCP {
		return 0, ""
	}
	info := KrpcInfo{}
	if err := info.parse(payload, true); err != nil || info.MsgType != sdk.DirectionRequest {
		return 0, ""
	}
	return KRPC_PROTOCOL, "KRPC"
}

func (p parser) OnParsePayload(ctx *sdk.ParseCtx) sdk.Action {
	if ctx.L7 != KRPC_PROTOCOL {
		return sdk.ActionNext()
	}

	payload, err := ctx.GetPayload()
	if err != nil {
		return sdk.ActionAbortWithErr(err)
	}
	info := KrpcInfo{}
	if err := info.parse(payload, false); err != nil {
		return sdk.ActionAbort()
	}

	var (
		reqid = uint32(info.Sequence)
		trace *sdk.Trace
		req   *sdk.Request
		resp  *sdk.Response
	)

	switch ctx.Direction {
	case sdk.DirectionRequest:
		req = &sdk.Request{
			ReqType:  strconv.FormatInt(int64(info.MsgId), 10),
			Resource: strconv.FormatInt(int64(info.ServId), 10),
			Endpoint: fmt.Sprintf("%d/%d", info.ServId, info.MsgId),
		}
	case sdk.DirectionResponse:
		resp = &sdk.Response{
			Status: &info.Status,
			Code:   &info.RetCode,
		}
	default:
		panic("unreachable")
	}

	if info.TraceId != "" || info.SpanId != "" || info.ParentSpanId != "" {
		trace = &sdk.Trace{
			TraceID:      info.TraceId,
			SpanID:       info.SpanId,
			ParentSpanID: info.ParentSpanId,
		}
	}

	return sdk.ParseActionAbortWithL7Info([]*sdk.L7ProtocolInfo{

		{
			RequestID: &reqid,
			Req:       req,
			Resp:      resp,
			Trace:     trace,
		},
	})
}

func (p parser) HookIn() []sdk.HookBitmap {
	return []sdk.HookBitmap{
		sdk.HOOK_POINT_PAYLOAD_PARSE,
	}
}

//go:generate mkdir -p ./pb
//go:generate protoc --go-plugin_out=./pb --go-plugin_opt=paths=source_relative ./krpc_meta.proto
func main() {
	sdk.Info("krpc wasm plugin load")
	sdk.SetParser(parser{})
}
