package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"

	"github.com/deepflowio/deepflow-wasm-go-sdk/example/go_http2_uprobe/pb"
	"github.com/deepflowio/deepflow-wasm-go-sdk/sdk"
	_ "github.com/wasilibs/nottinygc"
)

//go:generate mkdir -p ./pb
//go:generate protoc --go_out=./pb ./pb.proto
func main() {
	sdk.SetParser(parser{})
	sdk.Warn("plugin loaded")
}

const (
	GO_HTTP2_EBPF_PROTOCOL = 1
)

type parser struct {
	sdk.DefaultParser
}

func (p parser) OnHttpReq(ctx *sdk.HttpReqCtx) sdk.Action {
	return sdk.ActionNext()
}

func (p parser) OnHttpResp(ctx *sdk.HttpRespCtx) sdk.Action {
	return sdk.ActionNext()
}

func (p parser) OnCheckPayload(ctx *sdk.ParseCtx) (protoNum uint8, protoStr string, direction uint8) {
	if ctx.EbpfType != sdk.EbpfTypeGoHttp2Uprobe && ctx.EbpfType != sdk.EbpfTypeGoHttp2UprobeDATA {
		return 0, "", 0
	}
	payload, err := ctx.GetPayload()
	if err != nil {
		return 0, "", 0
	}
	if _, _, _, err := parseHeader(payload); err != nil {
		return 0, "", 0
	}
	return GO_HTTP2_EBPF_PROTOCOL, "ebpf_go_http2", 0
}

func (p parser) OnParsePayload(ctx *sdk.ParseCtx) sdk.Action {
	if ctx.L7 != GO_HTTP2_EBPF_PROTOCOL {
		return sdk.ActionNext()
	}
	payload, err := ctx.GetPayload()
	if err != nil {
		return sdk.ActionAbortWithErr(err)
	}
	defaultStatus := sdk.RespStatusOk
	switch ctx.EbpfType {
	case sdk.EbpfTypeGoHttp2Uprobe:
		streamID, key, val, err := parseHeader(payload)
		if err != nil {
			return sdk.ActionAbortWithErr(err)
		}

		info := &sdk.L7ProtocolInfo{
			RequestID: &streamID,
			Req:       &sdk.Request{},
			Resp: &sdk.Response{
				Status: &defaultStatus,
			},
			ProtocolMerge: true,
		}
		if err := onHeader(info, key, val); err != nil {
			return sdk.ActionAbortWithErr(err)
		}
		return sdk.ParseActionAbortWithL7Info([]*sdk.L7ProtocolInfo{info})
	case sdk.EbpfTypeGoHttp2UprobeDATA:
		streamID, data, err := parseData(payload)
		if err != nil {
			return sdk.ActionAbortWithErr(err)
		}

		var (
			attr     = []sdk.KeyVal{}
			traceID  string
			infoReq  *sdk.Request
			infoResp *sdk.Response
		)

		switch ctx.Direction {
		case sdk.DirectionResponse:
			resp := &pb.OrderResponse{}
			if err := resp.UnmarshalVT(data); err != nil {
				return sdk.ActionAbort()
			}

			attr = []sdk.KeyVal{
				{
					Key: "msg",
					Val: resp.Msg,
				},
			}
			infoResp = &sdk.Response{
				Status: &defaultStatus,
			}
		case sdk.DirectionRequest:
			req := &pb.OrderRequest{}
			if err := req.UnmarshalVT(data); err != nil {
				return sdk.ActionAbort()
			}
			attr = []sdk.KeyVal{
				{
					Key: "business_id",
					Val: req.BusinessId,
				},
			}
			traceID = req.BusinessId
			infoReq = &sdk.Request{}
		default:
			return sdk.ActionAbort()
		}

		info := &sdk.L7ProtocolInfo{
			RequestID:     &streamID,
			Req:           infoReq,
			Resp:          infoResp,
			Kv:            attr,
			ProtocolMerge: true,
			IsEnd:         true,
		}
		if traceID != "" {
			info.Trace = &sdk.Trace{
				TraceID: traceID,
			}
		}
		return sdk.ParseActionAbortWithL7Info([]*sdk.L7ProtocolInfo{info})
	default:
		return sdk.ActionNext()

	}
}

func (p parser) HookIn() []sdk.HookBitmap {
	return []sdk.HookBitmap{
		sdk.HOOK_POINT_PAYLOAD_PARSE,
	}
}

/*
fd(4 bytes)
stream id (4 bytes)
header key len (4 bytes)
header value len (4 bytes)
header key value (xxx bytes)
header value value (xxx bytes)
*/
func parseHeader(payload []byte) (uint32, string, string, error) {
	if len(payload) < 16 {
		return 0, "", "", errors.New("header payload too short")
	}

	streamID := binary.LittleEndian.Uint32(payload[4:8])
	keyLen := int(binary.LittleEndian.Uint32(payload[8:12]))
	valLen := int(binary.LittleEndian.Uint32(payload[12:16]))
	if keyLen < 0 || keyLen < 0 || keyLen+valLen+16 > len(payload) {
		return 0, "", "", fmt.Errorf("header kv length too short, key len: %d, val len: %d, payload len: %d", keyLen, valLen, len(payload))
	}

	return streamID, string(payload[16 : 16+keyLen]), string(payload[16+keyLen : 16+keyLen+valLen]), nil
}

/*
stream id (4 bytes)
data len (4 bytes)
unknown 5 bytes
pb data ($data_len - 5 bytes)
*/
func parseData(payload []byte) (uint32, []byte, error) {
	if len(payload) < 13 {
		return 0, nil, errors.New("data less than 8 bytes")
	}
	streamID := binary.LittleEndian.Uint32(payload[:4])
	dataLen := int(binary.LittleEndian.Uint32(payload[4:8]))
	if dataLen < 5 {
		return 0, nil, errors.New("data length too short")
	}
	if dataLen+8 > len(payload) {
		return 0, nil, fmt.Errorf("data payload too short, data len %d, payload len %d", dataLen, len(payload))
	}
	return streamID, payload[13:], nil

}

func onHeader(info *sdk.L7ProtocolInfo, key string, val string) error {
	switch key {
	case ":method":
		info.Req.ReqType = val
	case ":path":
		info.Req.Resource = val
	case ":host":
		info.Req.Domain = val
	case ":status":
		code, err := strconv.ParseInt(val, 10, 16)
		if err != nil {
			return err
		}
		statusCode := int32(code)
		info.Resp.Code = &statusCode

		var getStatus = func(statusCode int32) sdk.RespStatus {
			if statusCode >= 200 && statusCode < 400 {
				return sdk.RespStatusOk
			}
			if statusCode >= 400 && statusCode < 500 {
				return sdk.RespStatusClientErr
			}
			return sdk.RespStatusServerErr
		}
		status := getStatus(statusCode)
		info.Resp.Status = &status
	}
	return nil
}
