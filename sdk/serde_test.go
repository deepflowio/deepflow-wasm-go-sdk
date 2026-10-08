package sdk

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/deepflowio/deepflow-wasm-go-sdk/sdk/pb"
	"google.golang.org/protobuf/proto"
)

func TestSerializeL7ProtocolInfoXRequestIDs(t *testing.T) {
	for _, direction := range []Direction{DirectionRequest, DirectionResponse} {
		for _, tc := range []struct {
			name  string
			trace Trace
		}{
			{name: "legacy", trace: Trace{XRequestID: "legacy"}},
			{name: "request_id", trace: Trace{XRequestID0: "request-id"}},
			{name: "response_id", trace: Trace{XRequestID1: "response-id"}},
			{name: "both", trace: Trace{XRequestID0: "request-id", XRequestID1: "response-id"}},
			{name: "legacy_and_explicit", trace: Trace{XRequestID: "legacy", XRequestID0: "request-id", XRequestID1: "response-id"}},
			{name: "empty", trace: Trace{}},
		} {
			t.Run(fmt.Sprintf("%s/direction_%d", tc.name, direction), func(t *testing.T) {
				info := &L7ProtocolInfo{Req: &Request{}, Resp: &Response{}, Trace: &tc.trace}
				encoded := serializeL7ProtocolInfo([]*L7ProtocolInfo{info}, direction)
				if len(encoded) < 4 || string(encoded[2:4]) != "PB" || int(binary.BigEndian.Uint16(encoded[:2])) != len(encoded)-2 {
					t.Fatalf("invalid serialized frame: %x", encoded)
				}
				var msg pb.AppInfo
				if err := proto.Unmarshal(encoded[4:], &msg); err != nil {
					t.Fatal(err)
				}
				if (direction == DirectionRequest && msg.GetReq() == nil) || (direction == DirectionResponse && msg.GetResp() == nil) {
					t.Fatal("request/response type was not preserved")
				}
				trace := msg.GetTrace()
				if trace.GetXRequestId() != tc.trace.XRequestID || trace.GetXRequestId_0() != tc.trace.XRequestID0 || trace.GetXRequestId_1() != tc.trace.XRequestID1 {
					t.Fatalf("request IDs were not preserved: %v", trace)
				}
				if (trace.XRequestId_0 != nil) != (tc.trace.XRequestID0 != "") || (trace.XRequestId_1 != nil) != (tc.trace.XRequestID1 != "") {
					t.Fatalf("empty explicit IDs must be omitted: %v", trace)
				}
				var vtMsg pb.AppInfo
				if err := vtMsg.UnmarshalVT(encoded[4:]); err != nil {
					t.Fatal(err)
				}
				if !proto.Equal(&msg, &vtMsg) {
					t.Fatal("standard protobuf and VT decoding disagree")
				}
			})
		}
	}
}
