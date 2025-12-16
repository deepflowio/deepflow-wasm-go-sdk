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

import (
	"encoding/binary"
	"net"
	"strconv"

	"github.com/deepflowio/deepflow-wasm-go-sdk/sdk/pb"
	"google.golang.org/protobuf/proto"
)

const PAGE_SIZE = 65536
const PARSE_PARAM_BUF_SIZE = 1024
const HTTP_REQ_BUF_SIZE = PAGE_SIZE
const HTTP_RESP_BUF_SIZE = 3
const CUSTOM_MESSAGE_BUF_SIZE = PAGE_SIZE
const KV_SERDE_BUF_SIZE = PAGE_SIZE
const L7_INFO_BUF_SIZE = PAGE_SIZE

func writeStr(s string, buf []byte, offset *int) bool {
	l, off := len(s), *offset

	if off+2+l > len(buf) {
		Error("serialize string fail, buf size not enough")
		return false
	}

	binary.BigEndian.PutUint16(buf[off:off+2], uint16(l))
	off += 2

	copy(buf[off:off+l], s)
	off += l
	*offset = off
	return true
}

/*
serial format as follows, be encoding

ip type:     1 byte, 4 and 6 indicate  ipv4/ipv6
src_ip:      4/16 bytes
dst_ip:      4/16 bytes
src_port:    2 bytes
dst_port:    2 bytes

l4 protocol: 1 byte, 6/17 indicate udp/tcp
l7 protocol: 1 byte

ebpf type:   1 byte

time:        8 bytes

direction:   1 byte, 0/1 indicate c2s/s2c

proc name len:  1 byte

proc name: 		$(proc name len) len

flow_id:     8 bytes

buf_size:    2 bytes
*/
func deserializeParseCtx(b []byte) *ParseCtx {
	ctx := &ParseCtx{}
	off := 0
	if off+1 > len(b) {
		Error("deserialize parse ctx ip type fail")
		return nil
	}
	switch b[0] {
	case 4:
		off += 1
		if off+12 > len(b) {
			Error("deserialize parse ctx ipv4 fail")
			return nil
		}
		ctx.SrcIP = net.IPAddr{
			IP: b[1:5],
		}
		ctx.DstIP = net.IPAddr{
			IP: b[5:9],
		}
		ctx.SrcPort = binary.BigEndian.Uint16(b[9:11])
		ctx.DstPort = binary.BigEndian.Uint16(b[11:13])
		off += 12
	case 6:
		off += 1
		if off+36 > len(b) {
			Error("deserialize parse ctx ipv6 fail")
			return nil
		}

		ctx.SrcIP = net.IPAddr{
			IP: b[1:17],
		}
		ctx.DstIP = net.IPAddr{
			IP: b[17:33],
		}
		ctx.SrcPort = binary.BigEndian.Uint16(b[33:35])
		ctx.DstPort = binary.BigEndian.Uint16(b[35:37])
		off += 36
	default:
		Error("receive unexpected ip type " + strconv.FormatInt(int64(b[0]), 10))
		return nil
	}

	if off+13 > len(b) {
		Error("deserialize parse ctx fail")
		return nil
	}

	l4 := L4Protocol(b[off])
	switch l4 {
	case UDP, TCP:
		ctx.L4 = l4
	default:
		Error("receive unexpected l4 protocol " + strconv.Itoa(int(l4)))
		return nil
	}

	ctx.L7 = b[off+1]
	off += 2

	ebpfType := EbpfType(b[off])
	switch ebpfType {
	case EbpfTypeTracePoint,
		EbpfTypeTlsUprobe,
		EbpfTypeGoHttp2UprobeDATA,
		EbpfTypeGoHttp2Uprobe,
		EbpfTypeNone:
		ctx.EbpfType = ebpfType
	default:
		Error("receive unexpected ebpf type " + strconv.Itoa(int(ebpfType)))
		return nil
	}
	off += 1

	ctx.Time = binary.BigEndian.Uint64(b[off : off+8])
	off += 8

	direction := Direction(b[off])
	switch direction {
	case DirectionRequest, DirectionResponse:
		ctx.Direction = direction
	default:
		Error("receive unexpected direction " + strconv.Itoa(int(direction)))
		return nil
	}
	off += 1

	procNameLen := int(b[off])
	off += 1
	if off+procNameLen > len(b) {
		Error("deserialize parse ctx proc name fail")
		return nil
	}
	if procNameLen != 0 {
		ctx.ProcName = string(b[off : off+procNameLen])
		off += procNameLen
	}

	// flow id
	if off+8 > len(b) {
		Error("deserialize parse ctx flow id fail")
		return nil
	}

	ctx.FlowID = binary.BigEndian.Uint64(b[off : off+8])
	off += 8

	// buffer size
	if off+2 > len(b) {
		Error("deserialize parse ctx flow id fail")
		return nil
	}

	ctx.BufSize = binary.BigEndian.Uint16(b[off : off+2])
	off += 2

	return ctx
}

/*
hook_point:	  2 byte
type_code:	  4 byte
protobuf_len: 4 byte
protobuf:	  $(protobuf_len) byte
*/
func deserializeCustomMessageCtx(paramBuf, CustomMessageBuf []byte) *CustomMessageCtx {
	msgBufLen := len(CustomMessageBuf)
	if msgBufLen < 10 {
		return nil
	}

	baseCtx := deserializeParseCtx(paramBuf)
	if baseCtx == nil {
		return nil
	}
	ctx := &CustomMessageCtx{
		BaseCtx: *baseCtx,
	}
	ctx.HookPoint = binary.BigEndian.Uint16(CustomMessageBuf[:2])
	ctx.TypeCode = binary.BigEndian.Uint32(CustomMessageBuf[2:6])
	len := binary.BigEndian.Uint32(CustomMessageBuf[6:10])
	if len+10 > uint32(msgBufLen) {
		Error("CustomMessageCtx deserialize fail")
		return nil
	}
	ctx.Payload = CustomMessageBuf[10 : 10+len]

	return ctx
}

/*
path len:  2 byte
path:      $(path len) byte

host len:     2 byte
host:         $(host len) byte

ua len:     2 byte
ua:         $(ua len) byte

referer len:  2 byte
referer:      $(referer) byte
*/
func deserializeHttpReqCtx(paramBuf, httpReqBuf []byte) *HttpReqCtx {
	reqBufLen := len(httpReqBuf)
	if reqBufLen < 8 {
		return nil
	}

	baseCtx := deserializeParseCtx(paramBuf)
	if baseCtx == nil {
		return nil
	}
	ctx := &HttpReqCtx{
		BaseCtx: *baseCtx,
	}
	off := 0
	s := [4]string{}

	for i := 0; i < len(s); i++ {
		if off+2 > reqBufLen {
			Error("httpReqCtx deserialize fail")
			return nil
		}
		strLen := int(binary.BigEndian.Uint16(httpReqBuf[off : off+2]))
		off += 2
		if strLen == 0 {
			continue
		}
		if off+strLen > reqBufLen {
			Error("httpReqCtx deserialize fail")
			return nil
		}
		s[i] = string(httpReqBuf[off : off+strLen])
		off += strLen
	}

	ctx.Path = s[0]
	ctx.Host = s[1]
	ctx.UserAgent = s[2]
	ctx.Referer = s[3]

	return ctx

}

/*
code:     2 bytes
status:   1 byte
*/
func deserializeHttpRespCtx(paramBuf, httpRespBuf []byte) *HttpRespCtx {
	respBufLen := len(httpRespBuf)
	if respBufLen < 3 {
		return nil
	}

	status := RespStatus(httpRespBuf[2])
	switch status {
	case RespStatusOk, RespStatusTimeout, RespStatusServerErr, RespStatusClientErr, RespStatusUnknown:
	default:
		Error("httpRespBuf recv unknown status: %d", status)
		return nil
	}

	baseCtx := deserializeParseCtx(paramBuf)
	if baseCtx == nil {
		return nil
	}
	ctx := &HttpRespCtx{
		BaseCtx: *baseCtx,
		Status:  status,
		Code:    binary.BigEndian.Uint16(httpRespBuf[:2]),
	}
	return ctx
}

func serializeL7ProtocolInfo(infos []*L7ProtocolInfo, direction Direction) []byte {
	buf := [L7_INFO_BUF_SIZE]byte{}
	off := 0

	checkLen := func(size int) bool {
		if off+size > len(buf) {
			Error("serialize l7ProtocolInfo fail, data too large, serialize size must less than 65536 bytes")
			return false
		}
		return true
	}

	for _, info := range infos {
		start := off
		// leave 2 bytes as length, 2 bytes as magic (PB)
		off += 4

		var msg pb.AppInfo

		if info.ReqLen != nil {
			msg.ReqLen = proto.Uint32(uint32(*info.ReqLen))
		}

		if info.RespLen != nil {
			msg.RespLen = proto.Uint32(uint32(*info.RespLen))
		}

		if info.RequestID != nil {
			msg.RequestId = proto.Uint32(uint32(*info.RequestID))
		}

		if info.IsAsync != nil {
			msg.IsAsync = proto.Bool(bool(*info.IsAsync))
		}

		if info.IsReversed != nil {
			msg.IsReversed = proto.Bool(bool(*info.IsReversed))
		}

		switch direction {
		case DirectionRequest:
			if info.Req == nil {
				Error("c2s data but request is nil")
				return nil
			}
			msg.Info = &pb.AppInfo_Req{
				Req: &pb.AppRequest{
					Version:  proto.String(info.Req.Version),
					Type:     proto.String(info.Req.ReqType),
					Endpoint: proto.String(info.Req.Endpoint),
					Domain:   proto.String(info.Req.Domain),
					Resource: proto.String(info.Req.Resource),
				},
			}
		case DirectionResponse:
			if info.Resp == nil {
				Error("s2c data but resp is nil")
				return nil
			}

			var status pb.AppRespStatus
			if info.Resp.Status == nil {
				status = pb.AppRespStatus_RESP_UNKNOWN
			} else {
				switch *info.Resp.Status {
				case RespStatusOk:
					status = pb.AppRespStatus_RESP_OK
				case RespStatusTimeout:
					status = pb.AppRespStatus_RESP_TIMEOUT
				case RespStatusServerErr:
					status = pb.AppRespStatus_RESP_SERVER_ERROR
				case RespStatusClientErr:
					status = pb.AppRespStatus_RESP_CLIENT_ERROR
				case RespStatusUnknown:
					status = pb.AppRespStatus_RESP_UNKNOWN
				}
			}
			resp := pb.AppResponse{
				Status:    &status,
				Result:    proto.String(info.Resp.Result),
				Exception: proto.String(info.Resp.Exception),
			}

			if info.Resp.Code != nil {
				resp.Code = proto.Int32(*info.Resp.Code)
			}
			msg.Info = &pb.AppInfo_Resp{
				Resp: &resp,
			}
		}

		msg.ProtocolStr = proto.String(info.L7ProtocolStr)

		if info.Trace != nil {
			msg.Trace = &pb.AppTrace{
				TraceId:         proto.String(info.Trace.TraceID),
				SpanId:          proto.String(info.Trace.SpanID),
				ParentSpanId:    proto.String(info.Trace.ParentSpanID),
				XRequestId:      proto.String(info.Trace.XRequestID),
				HttpProxyClient: proto.String(info.Trace.HttpProxyClient),
				TraceIds:        info.Trace.TraceIDs,
			}
		}

		for _, kv := range info.Kv {
			msg.Attributes = append(msg.Attributes, &pb.KeyVal{
				Key: kv.Key,
				Val: kv.Val,
			})
		}

		msg.BizType = proto.Uint32(uint32(info.BizType))
		msg.BizCode = proto.String(info.BizCode)
		msg.BizScenario = proto.String(info.BizScenario)

		serSize := msg.SizeVT()

		if !checkLen(serSize) {
			return nil
		}
		_, err := msg.MarshalToVT(buf[start+off:])
		if err != nil {
			Error("serialize l7ProtocolInfo failed: %s", err)
			return nil
		}
		off += serSize

		binary.BigEndian.PutUint16(buf[start:], uint16(serSize+2))
		// magic
		copy(buf[start+2:], "PB")
	}
	return buf[:off]
}
