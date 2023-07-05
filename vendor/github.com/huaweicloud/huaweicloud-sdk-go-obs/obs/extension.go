// Copyright 2019 Huawei Technologies Co.,Ltd.
// Licensed under the Apache License, Version 2.0 (the "License"); you may not use
// this file except in compliance with the License.  You may obtain a copy of the
// License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software distributed
// under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
// CONDITIONS OF ANY KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations under the License.

package obs

import (
	"fmt"
	"strconv"
	"strings"
)

type extensionOptions interface{}
type extensionHeaders func(headers map[string][]string, isObs bool) error

func setHeaderPrefix(key string, value string) extensionHeaders {
	return func(headers map[string][]string, isObs bool) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("set header %s with empty value", key)
		}
		setHeaders(headers, key, []string{value}, isObs)
		return nil
	}
}

// WithReqPaymentHeader sets header for requester-pays
func WithReqPaymentHeader(requester PayerType) extensionHeaders {
	return setHeaderPrefix(REQUEST_PAYER, string(requester))
}

func WithTrafficLimitHeader(trafficLimit int64) extensionHeaders {
	return setHeaderPrefix(TRAFFIC_LIMIT, strconv.FormatInt(trafficLimit, 10))
}

func WithCallbackHeader(callback string) extensionHeaders {
	return setHeaderPrefix(CALLBACK, string(callback))
}

func WithCustomHeader(key string, value string) extensionHeaders {
	return func(headers map[string][]string, isObs bool) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("set header %s with empty value", key)
		}
		headers[key] = []string{value}
		return nil
	}
}
