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
	"errors"
	"net"
)

const (
	UDP L4Protocol = 17
	TCP L4Protocol = 6
)

const (
	DirectionRequest  Direction = 0
	DirectionResponse Direction = 1
)

const (
	EbpfTypeTracePoint        EbpfType = 0
	EbpfTypeTlsUprobe         EbpfType = 1
	EbpfTypeGoHttp2Uprobe     EbpfType = 2
	EbpfTypeGoHttp2UprobeDATA EbpfType = 5
	EbpfTypeNone              EbpfType = 255
)

const (
	RespStatusOk        RespStatus = 0
	RespStatusNotExist  RespStatus = 2
	RespStatusServerErr RespStatus = 3
	RespStatusClientErr RespStatus = 4
)

type L4Protocol uint16
type Direction uint8
type EbpfType uint8
type RespStatus uint8

type HttpReqCtx struct {
	BaseCtx   ParseCtx
	Path      string
	Host      string
	UserAgent string
	Referer   string
}

type HttpRespCtx struct {
	BaseCtx ParseCtx
	Code    uint16
	Status  RespStatus
}

const (
	ProtocolParse uint16 = 0
	SessionFilter uint16 = 1
	Sampling      uint16 = 2
)

type CustomMessageCtx struct {
	BaseCtx   ParseCtx
	HookPoint uint16
	TypeCode  uint32
	Payload   []byte
}

func (ctx *CustomMessageCtx) CheckParseProtocol(protocol uint16, isRequest bool) bool {
	if isRequest {
		return ctx.HookPoint == ProtocolParse && ctx.TypeCode == uint32(protocol)
	}
	return false
}

type ParseCtx struct {
	SrcIP     net.IPAddr
	SrcPort   uint16
	DstIP     net.IPAddr
	DstPort   uint16
	L4        L4Protocol
	L7        uint8
	EbpfType  EbpfType
	Time      uint64 // micro second
	Direction Direction
	// only EbpfType is not EbpfTypeNone will not empty
	ProcName string
	FlowID   uint64
	BufSize  uint16
	payload  []byte
}

func (p *ParseCtx) GetPayload() ([]byte, error) {
	if p.payload != nil {
		return p.payload, nil
	}
	if int(p.BufSize) > PAGE_SIZE {
		Warn("payload buffer size %d greater than page size", p.BufSize)
	}
	payload := make([]byte, p.BufSize)
	payloadSize := vmReadPayload(&payload[0], len(payload))
	if payloadSize < 0 {
		return nil, errors.New("read payload fail")
	}
	p.payload = payload[:payloadSize]
	return p.payload, nil
}

type Request struct {
	ReqType  string
	Domain   string
	Resource string
	Endpoint string
}

type Response struct {
	Status    *RespStatus
	Code      *int32
	Result    string
	Exception string
}

type Trace struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
}

type L7ProtocolInfo struct {
	ReqLen    *int
	RespLen   *int
	RequestID *uint32
	Req       *Request
	Resp      *Response
	Trace     *Trace
	Kv        []KeyVal
	// cache the log in session merge and merge multi times until request end and response end
	ProtocolMerge bool
	// request/response end
	IsEnd         bool
	BizType       uint8
	L7ProtocolStr string
}
