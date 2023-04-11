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

import "fmt"

type LogLevel uint8

const (
	LogLevelInfo  LogLevel = 0
	LogLevelWarn  LogLevel = 1
	LogLevelError LogLevel = 2
)

func log(s string, level LogLevel) {
	if len(s) == 0 {
		return
	}
	wasmLog(&[]byte(s)[0], len(s), uint8(level))
}

func Info(s string, arg ...interface{}) {
	log(fmt.Sprintf(s, arg...), LogLevelInfo)
}

func Warn(s string, arg ...interface{}) {
	log(fmt.Sprintf(s, arg...), LogLevelWarn)
}

func Error(s string, arg ...interface{}) {
	log(fmt.Sprintf(s, arg...), LogLevelError)
}
