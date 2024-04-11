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
	case RespStatusOk, RespStatusNotExist, RespStatusServerErr, RespStatusClientErr:
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

/*
(

	key len: 2 bytes
	key:     $(key len) bytes

	val len: 2 bytes
	val:     $(val len) bytes

) x n
*/
func serializeKV(attr []KeyVal, buf []byte, offset *int) bool {

	off, count := *offset, 0

	for _, v := range attr {
		if writeStr(v.Key, buf, &off) && writeStr(v.Val, buf, &off) {
			count += 1
			continue
		}
		break
	}
	if count == 0 {
		Error("serialize kv fail")
		return false
	}

	if count != len(attr) {
		Warn("kv not all serialize")
	}
	*offset = off
	return true
}

/*
(

	info len: 2 bytes

	req len:  4	bytes: | 1 bit: is nil? | 31bit length |

	resp len:  4 bytes: | 1 bit: is nil? | 31bit length |

	has request id: 1 bytes:  0 or 1

	if has request id:

		request	id: 4 bytes

	if direction is c2s:

		req

	if direction is s2c:

		resp

	l7_protocol_str len: 2 bytes
	l7_protocol_str:     $(l7_protocol_str len) bytes

	need_protocol_merge: 1 byte, the msb indicate is need protocol merge, the lsb indicate is end, such as 1 000000 1

	has trace: 1 byte

	if has trace:

		trace_id, span_id, parent_span_id
		(

		key len: 2 bytes
		key:     $(key len) bytes

		val len: 2 bytes
		val:     $(val len) bytes

		) x 3


	has kv:  1 byte
	if has kv
		(
			key len: 2 bytes
			key:     $(key len) bytes

			val len: 2 bytes
			val:     $(val len) bytes

		) x len(kv)

	biz type: 1 byte

) x len(infos)
*/
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
		// leave 2 bytes as length
		off += 2

		// serialize req len
		if !checkLen(4) {
			return nil
		}
		if info.ReqLen == nil {
			binary.BigEndian.PutUint32(buf[off:off+4], 0)
		} else {
			binary.BigEndian.PutUint32(buf[off:off+4], uint32(*info.ReqLen)|(1<<31))
		}
		off += 4

		// serialize resp len
		if !checkLen(4) {
			return nil
		}
		if info.RespLen == nil {
			binary.BigEndian.PutUint32(buf[off:off+4], 0)
		} else {
			binary.BigEndian.PutUint32(buf[off:off+4], uint32(*info.RespLen)|(1<<31))
		}
		off += 4

		// serialize request id
		if info.RequestID != nil {
			if !checkLen(5) {
				return nil
			}
			buf[off] = 1
			off += 1
			binary.BigEndian.PutUint32(buf[off:off+4], *info.RequestID)
			off += 4
		} else {
			if !checkLen(1) {
				return nil
			}
			buf[off] = 0
			off += 1
		}

		// serialize req/resp
		size := 0
		switch direction {
		case DirectionRequest:
			if info.Req == nil {
				Error("c2s data but request is nil")
				return nil
			}
			size = serializeL7InfoReq(info.Req, buf[off:])
		case DirectionResponse:
			if info.Resp == nil {
				Error("s2c data but resp is nil")
				return nil
			}
			size = serializeL7InfoResp(info.Resp, buf[off:])
		}

		if size == 0 {
			Error("serialize L7ProtocolInfo req or resp fail")
			return nil
		}
		off += size

		// serialize l7_protocol_str
		if !writeStr(info.L7ProtocolStr, buf[:], &off) {
			Error("serialize L7ProtocolInfo l7_protocol_str fail")
			return nil
		}

		// serialize need_merge_protocol
		if !checkLen(1) {
			Error("serialize L7ProtocolInfo `ProtocolMerge` fail")
			return nil
		}
		var needProtocolMerge byte = 0
		if info.ProtocolMerge {
			needProtocolMerge = 1 << 7
			if info.IsEnd {
				needProtocolMerge |= 1
			}
		}
		buf[off] = needProtocolMerge
		off += 1

		// serialize trace info
		if !checkLen(1) {
			return nil
		}
		if info.Trace == nil {
			buf[off] = 0
			off += 1
		} else {
			buf[off] = 1
			off += 1
			if !(writeStr(info.Trace.TraceID, buf[:], &off) &&
				writeStr(info.Trace.SpanID, buf[:], &off) &&
				writeStr(info.Trace.ParentSpanID, buf[:], &off)) {
				Error("serialize L7ProtocolInfo trace fail")
				return nil
			}
		}

		// serialize kv
		if !checkLen(1) {
			return nil
		}
		if len(info.Kv) != 0 {
			buf[off] = 1
			off += 1
			if !serializeKV(info.Kv, buf[:], &off) {
				return nil
			}
		} else {
			buf[off] = 0
			off += 1
		}

		// serialize biz type
		if !checkLen(1) {
			return nil
		}
		buf[off] = info.BizType
		off += 1

		binary.BigEndian.PutUint16(buf[start:start+2], uint16(off-start-2))
	}
	return buf[:off]
}

/*
ReqType, Endpoint, Domain, Resource
(

	len: 2 bytes
	val: $(len) bytes

) x 4
*/
func serializeL7InfoReq(req *Request, buf []byte) int {
	off := 0
	if writeStr(req.ReqType, buf, &off) &&
		writeStr(req.Endpoint, buf, &off) &&
		writeStr(req.Domain, buf, &off) &&
		writeStr(req.Resource, buf, &off) {
		return off
	}
	return 0
}

/*
status:    1 byte,
has code:  1 byte, 0 or 1,

if has code:

	code:  4 bytes,

Result, Exception
(

	len: 2 bytes
	val: $(len) bytes

) x 2
*/
func serializeL7InfoResp(resp *Response, buf []byte) int {
	off := 0
	if off+1 > len(buf) {
		return 0
	}
	if resp.Status == nil {
		status := RespStatusNotExist
		resp.Status = &status
	}
	buf[off] = byte(*resp.Status)
	off += 1
	if resp.Code != nil {
		buf[off] = 1
		off += 1
		if off+4 > len(buf) {
			return 0
		}
		binary.BigEndian.PutUint32(buf[off:off+4], uint32(*resp.Code))
		off += 4
	} else {
		buf[off] = 0
		off += 1
	}

	if writeStr(resp.Result, buf, &off) &&
		writeStr(resp.Exception, buf, &off) {
		return off
	}
	return 0
}
