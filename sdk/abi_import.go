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

//go:wasm-module deepflow
//export wasm_log
func wasmLog(b *byte, length int, level uint8)

// return size, 0 indicate fail
//
//go:wasm-module deepflow
//export vm_read_ctx_base
func vmReadCtxBase(b *byte, length int) int

// return <0 indicate fail
//
//go:wasm-module deepflow
//export vm_read_payload
func vmReadPayload(b *byte, length int) int

// return size, 0 indicate fail
//
//go:wasm-module deepflow
//export vm_read_http_req_info
func vmReadHttpReqInfo(b *byte, length int) int

// return size, 0 indicate fail
//
//go:wasm-module deepflow
//export vm_read_http_resp_info
func vmReadHttpRespInfo(b *byte, length int) int

// return size, 0 indicate fail
//
//go:wasm-module deepflow
//export vm_read_custom_message_info
func vmReadCustomMessageInfo(b *byte, length int) int

//go:wasm-module deepflow
//export host_read_l7_protocol_info
func hostReadL7ProtocolInfo(b *byte, length int) bool

//go:wasm-module deepflow
//export host_read_http_result
func hostReadHttpResult(b *byte, length int) bool

//go:wasm-module deepflow
//export host_read_str_result
func hostReadStrResult(b *byte, length int) bool
