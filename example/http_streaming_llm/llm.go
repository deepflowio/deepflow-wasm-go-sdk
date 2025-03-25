package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/deepflowio/deepflow-wasm-go-sdk/sdk"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// 统计计算大模型TTFT,TPOT等指标
// 1. 首Token延迟: 即从输入到输出第一个Token的延迟, TTFT = respFirstChunkedTime - reqTime
// 2. 每个输出Token的延迟（不含首个Token）: 即从第二个输出Token开始的吐出速度，TPOT = (respLastChunkedTime - respFirstChunkedTime)/(totalToken-1)
// 3. 服务实时并发量: 大模型服务当前建立并在处理的长连接请求数量
type StreamInfo struct {
	reqTime uint64
	// 首次分块响应的时间
	respFirstChunkedTime uint64
	// 分块传输结束时间
	//respLastChunkedTime uint64
	totalToken uint64
	flag       int
}

/*
大模型http流式请求:

	POST /xxx/generate_stream HTTP/1.1
	Content-Type: applicatin/json
	{"inputs": "test", "parameters": {"do_sample": false, "max_new_tokens": 80}, "stream": True}

判断http流式请求:

	(1) /generate_stream
	(2) body stream=True
*/
func checker(payload []byte) (protoNum uint8, protoStr string) {
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(payload)))
	if err != nil {
		return 0, ""
	}

	query := req.URL.Path
	if strings.Contains(query, "/generate_stream") {
		sdk.Warn(fmt.Sprintf("check: %s", query))
		return 1, "http_stream"
	}
	return 0, ""
}

/*
大模型http流式响应
*/
func parser(payload []byte, flowid uint64) {
}

type llmParser struct {
	httpStream map[uint64]*StreamInfo
}

func (p *llmParser) HookIn() []sdk.HookBitmap {
	return []sdk.HookBitmap{
		// 表示协议的判断和解析
		sdk.HOOK_POINT_PAYLOAD_PARSE,
	}
}

func (p *llmParser) OnHttpReq(ctx *sdk.HttpReqCtx) sdk.Action {
	return sdk.ActionNext()
}

func (p *llmParser) OnHttpResp(ctx *sdk.HttpRespCtx) sdk.Action {
	return sdk.ActionNext()
}

func (p *llmParser) OnCheckPayload(baseCtx *sdk.ParseCtx) (protoNum uint8, protoStr string) {
	if baseCtx.EbpfType != sdk.EbpfTypeNone {
		return 0, ""
	}
	payload, err := baseCtx.GetPayload()
	if err != nil {
		//sdk.Error("get payload fail: %v", err)
		return 0, ""
	}

	// TODO 判断大模型流式请求
	if baseCtx.Direction == sdk.DirectionRequest {
		return checker(payload)
	}
	return 0, ""
}

func (p *llmParser) OnParsePayload(baseCtx *sdk.ParseCtx) sdk.Action {
	if baseCtx.L7 != 1 {
		return sdk.ActionNext()
	}
	payload, err := baseCtx.GetPayload()
	if err != nil {
		return sdk.ActionAbortWithErr(err)
	}

	var attr = []sdk.KeyVal{}
	var streamId string
	var flowId = baseCtx.FlowID
	if p.httpStream[flowId] == nil {
		p.httpStream[flowId] = &StreamInfo{}
	}

	switch baseCtx.Direction {
	case sdk.DirectionRequest:
		streamId = fmt.Sprintf("%s:%d->%s:%d %d", baseCtx.DstIP.String(), baseCtx.DstPort, baseCtx.SrcIP.String(), baseCtx.SrcPort, flowId)
		sdk.Warn("parse-req-start:" + streamId)
		req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(payload)))
		if err != nil {
			return sdk.ActionNext()
		}
		p.httpStream[flowId].reqTime = baseCtx.Time
		info := &sdk.L7ProtocolInfo{
			Req: &sdk.Request{
				Resource: req.URL.Path,
			},
			Resp: &sdk.Response{},
		}
		return sdk.ParseActionAbortWithL7Info([]*sdk.L7ProtocolInfo{info})
	case sdk.DirectionResponse:
		streamId = fmt.Sprintf("%s:%d->%s:%d %d", baseCtx.SrcIP.String(), baseCtx.SrcPort, baseCtx.DstIP.String(), baseCtx.DstPort, flowId)
		sdk.Warn("parse-resp-start:" + streamId)
		// 开始流式响应处理： 分块传输
		r := bufio.NewReader(bytes.NewReader(payload))
		bs, _, err := r.ReadLine()
		if err == io.EOF {
			sdk.Warn("parse-resp-end-01")
			return sdk.ActionNext()
		}
		regex := regexp.MustCompile(`^HTTP/[1-2]\.[01] \d{3} .*$`)
		if regex.MatchString(string(bs)) {
			// http响应状态行判断
			sdk.Warn("parse-resp-end-00")
			return sdk.ActionNext()
		}
		sdk.Warn(fmt.Sprintf("parse-resp:%s", string(bs)))
		// 结束流式响应处理
		if string(bs) == "0" {
			attr = []sdk.KeyVal{
				{
					Key: "ttft",
					Val: fmt.Sprintf("%d", p.httpStream[flowId].respFirstChunkedTime-p.httpStream[flowId].reqTime),
				},
				{
					Key: "tpot",
					Val: fmt.Sprintf("%d", (baseCtx.Time-p.httpStream[flowId].respFirstChunkedTime)/p.httpStream[flowId].totalToken),
				},
				{
					Key: "tokens",
					Val: fmt.Sprintf("%d", p.httpStream[flowId].totalToken),
				},
			}
			status := sdk.RespStatusOk
			code := int32(200)
			info := &sdk.L7ProtocolInfo{
				Req: &sdk.Request{},
				Resp: &sdk.Response{
					Status: &status,
					Code:   &code,
				},
				Kv: attr,
			}
			if _, exists := p.httpStream[flowId]; exists {
				delete(p.httpStream, flowId)
			}
			return sdk.ParseActionAbortWithL7Info([]*sdk.L7ProtocolInfo{info})
		}
		bs, _, err = r.ReadLine()
		if err == io.EOF {
			sdk.Warn("parse-resp-end-02")
			return sdk.ActionNext()
		}
		sdk.Warn(fmt.Sprintf("parse-resp:%d %s", baseCtx.Time, string(bs)))
		// TODO 判断响应首包
		if p.httpStream[flowId].flag == 0 {
			p.httpStream[flowId].flag = 1
			p.httpStream[flowId].respFirstChunkedTime = baseCtx.Time
			p.httpStream[flowId].totalToken = uint64(len(bs))
			return sdk.ActionNext()
		}
		p.httpStream[flowId].totalToken = p.httpStream[flowId].totalToken + uint64(len(bs))
		sdk.Warn(fmt.Sprintf("total-token:%d", p.httpStream[flowId].totalToken))
		bs, _, err = r.ReadLine()
		if err == io.EOF {
			sdk.Warn("parse-resp-end-03")
			if _, exists := p.httpStream[flowId]; exists {
				delete(p.httpStream, flowId)
			}
			return sdk.ActionNext()
		}
		sdk.Warn(fmt.Sprintf("parse-resp:%s", string(bs)))
		sdk.Warn("parse-resp-end-04")
		return sdk.ActionNext()
	default:
		return sdk.ActionNext()
	}
}

func main() {
	sdk.Warn("llm wasm plugin loaded")
	llm := &llmParser{
		httpStream: map[uint64]*StreamInfo{},
	}
	sdk.SetParser(llm)
}
