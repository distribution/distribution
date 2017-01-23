/**
 * Copyright (c) 2015 Guoyao Wu, All Rights Reserved
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * @file core.go
 * @author guoyao
 */

// Package bce defined a set of core data structure and functions for Baidu Cloud API.
package bce

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/guoyao/baidubce-sdk-go/util"
)

const (
	Version = "1.0.3"

	// ExpirationPeriodInSeconds 1800s is the default expiration period.
	ExpirationPeriodInSeconds = 1800
)

// DefaultUserAgent is the default value of http request UserAgent header.
//
// We can change it by specifying the UserAgent field of bce.Config.
var DefaultUserAgent = strings.Join([]string{
	"baidubce-sdk-go",
	Version,
	runtime.GOOS,
	runtime.Version(),
}, "/")

// Region contains all regions of Baidu Cloud.
var Region = map[string]string{
	"bj": "bj",
	"gz": "gz",
	"hk": "hk",
}

type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
}

func NewCredentials(AccessKeyID, secretAccessKey string) *Credentials {
	return &Credentials{AccessKeyID, secretAccessKey}
}

// Config contains all options for bce.Client.
type Config struct {
	*Credentials
	Region     string
	Endpoint   string
	APIVersion string
	Protocol   string
	UserAgent  string
	ProxyHost  string
	ProxyPort  int
	//ConnectionTimeoutInMillis time.Duration // default value: 10 * time.Second in http.DefaultTransport
	MaxConnections int           // default value: 2 in http.DefaultMaxIdleConnsPerHost
	Timeout        time.Duration // default value: 0 in http.Client
	RetryPolicy    RetryPolicy
	Checksum       bool
}

func NewConfig(credentials *Credentials) *Config {
	return &Config{
		Credentials: credentials,
		Region:      Region["bj"],
	}
}

// GetRegion gets region from bce.Config.
//
// If no region specified in bce.Config, the bj region will be return.
func (config *Config) GetRegion() string {
	region := config.Region

	if region == "" {
		region = Region["bj"]
	}

	return region
}

// GetUserAgent gets UserAgent from bce.Config.
//
// If no UserAgent specified in bce.Config, the bce.DefaultUserAgent will be return.
func (config *Config) GetUserAgent() string {
	userAgent := config.UserAgent

	if userAgent == "" {
		userAgent = DefaultUserAgent
	}

	return userAgent
}

// RetryPolicy defined an interface for retrying of bce.Client.
type RetryPolicy interface {
	GetMaxErrorRetry() int      // GetMaxErrorRetry specifies the max retry count.
	GetMaxDelay() time.Duration // GetMaxDelay specifies the max delay time for retrying.

	// GetDelayBeforeNextRetry specifies the delay time for next retry.
	GetDelayBeforeNextRetry(err error, retriesAttempted int) time.Duration
}

// DefaultRetryPolicy is the default implemention of interface bce.RetryPolicy.
type DefaultRetryPolicy struct {
	MaxErrorRetry int
	MaxDelay      time.Duration
}

func NewDefaultRetryPolicy(maxErrorRetry int, maxDelay time.Duration) *DefaultRetryPolicy {
	return &DefaultRetryPolicy{maxErrorRetry, maxDelay}
}

// GetMaxErrorRetry specifies the max retry count.
func (policy *DefaultRetryPolicy) GetMaxErrorRetry() int {
	return policy.MaxErrorRetry
}

// GetMaxDelay specifies the max delay time for retrying.
func (policy *DefaultRetryPolicy) GetMaxDelay() time.Duration {
	return policy.MaxDelay
}

// GetDelayBeforeNextRetry specifies the delay time for next retry.
func (policy *DefaultRetryPolicy) GetDelayBeforeNextRetry(err error, retriesAttempted int) time.Duration {
	if !policy.shouldRetry(err, retriesAttempted) {
		return -1
	}

	duration := (1 << uint(retriesAttempted)) * 300 * time.Millisecond

	if duration > policy.GetMaxDelay() {
		return policy.GetMaxDelay()
	}

	return duration
}

func (policy *DefaultRetryPolicy) shouldRetry(err error, retriesAttempted int) bool {
	if retriesAttempted > policy.GetMaxErrorRetry() {
		return false
	}

	if bceError, ok := err.(*Error); ok {
		if bceError.StatusCode == http.StatusInternalServerError {
			log.Println("Retry for internal server error.")
			return true
		}

		if bceError.StatusCode == http.StatusServiceUnavailable {
			log.Println("Retry for service unavailable.")
			return true
		}
	}

	return false
}

// SignOption contains all signature options of Baidu Cloud API.
type SignOption struct {
	Timestamp                 string
	ExpirationPeriodInSeconds int
	Headers                   map[string]string
	HeadersToSign             []string
	Credentials               *Credentials // for STS(Security Token Service) only
	headersToSignSpecified    bool
	initialized               bool
}

func NewSignOption(timestamp string, expirationPeriodInSeconds int,
	headers map[string]string, headersToSign []string) *SignOption {

	return &SignOption{timestamp, expirationPeriodInSeconds,
		headers, headersToSign, nil, len(headersToSign) > 0, false}
}

// CheckSignOption returns a new empty bce.SignOption instance if no option specified.
func CheckSignOption(option *SignOption) *SignOption {
	if option == nil {
		return &SignOption{}
	}

	return option
}

// AddHeadersToSign adds some headers for authentication process of Baidu Cloud API.
//
// For details, please refer https://cloud.baidu.com/doc/Reference/AuthenticationMechanism.html#1.1.20.E6.A6.82.E8.BF.B0
func (option *SignOption) AddHeadersToSign(headers ...string) {
	if option.HeadersToSign == nil {
		option.HeadersToSign = []string{}
		option.HeadersToSign = append(option.HeadersToSign, headers...)
	} else {
		for _, header := range headers {
			if !util.Contains(option.HeadersToSign, header, true) {
				option.HeadersToSign = append(option.HeadersToSign, header)
			}
		}
	}
}

// AddHeader adds a header and it's value for authentication process of Baidu Cloud API.
//
// For details, please refer https://cloud.baidu.com/doc/Reference/AuthenticationMechanism.html#1.1.20.E6.A6.82.E8.BF.B0
func (option *SignOption) AddHeader(key, value string) {
	if option.Headers == nil {
		option.Headers = make(map[string]string)
		option.Headers[key] = value
	}

	if !util.MapContains(option.Headers, generateHeaderValidCompareFunc(key)) {
		option.Headers[key] = value
	}
}

// AddHeaders adds some headers for authentication process of Baidu Cloud API.
//
// For details, please refer https://cloud.baidu.com/doc/Reference/AuthenticationMechanism.html#1.1.20.E6.A6.82.E8.BF.B0
func (option *SignOption) AddHeaders(headers map[string]string) {
	if headers == nil {
		return
	}

	if option.Headers == nil {
		option.Headers = make(map[string]string)
	}

	for key, value := range headers {
		option.AddHeader(key, value)
	}
}

func (option *SignOption) init() {
	if option.initialized {
		return
	}

	option.headersToSignSpecified = len(option.HeadersToSign) > 0

	if option.Timestamp == "" {
		option.Timestamp = util.TimeToUTCString(time.Now())
	}

	if option.ExpirationPeriodInSeconds <= 0 {
		option.ExpirationPeriodInSeconds = ExpirationPeriodInSeconds
	}

	if option.Headers == nil {
		option.Headers = make(map[string]string, 3)
	} else {
		util.MapKeyToLower(option.Headers)
	}

	util.SliceToLower(option.HeadersToSign)

	if !util.Contains(option.HeadersToSign, "host", true) {
		option.HeadersToSign = append(option.HeadersToSign, "host")
	}

	if !option.headersToSignSpecified {
		option.HeadersToSign = append(option.HeadersToSign, "x-bce-date")
		option.Headers["x-bce-date"] = option.Timestamp
	} else if util.Contains(option.HeadersToSign, "date", true) {
		if !util.MapContains(option.Headers, generateHeaderValidCompareFunc("date")) {
			option.Headers["date"] = time.Now().Format(time.RFC1123)
		} else {
			option.Headers["date"] = util.TimeStringToRFC1123(util.GetMapValue(option.Headers, "date", true))
		}
	} else {
		if !util.MapContains(option.Headers, generateHeaderValidCompareFunc("x-bce-date")) {
			option.Headers["x-bce-date"] = option.Timestamp
		}
	}

	option.initialized = true
}

func (option *SignOption) signedHeadersToString() string {
	headers := make([]string, 0, int(math.Max(float64(len(option.Headers)), float64(len(option.HeadersToSign)))))

	if option.headersToSignSpecified {
		headers = append(headers, option.HeadersToSign...)
	} else {
		for key, _ := range option.Headers {
			if isCanonicalHeader(key) {
				headers = append(headers, key)
			}
		}
	}

	sort.Strings(headers)

	return strings.Join(headers, ";")
}

// GenerateAuthorization generates authorization code for authorization process of Baidu Cloud API.
func GenerateAuthorization(credentials Credentials, req Request, option *SignOption) string {
	if option == nil {
		option = &SignOption{}
	}
	option.init()

	authorization := "bce-auth-v1/" + credentials.AccessKeyID
	authorization += "/" + option.Timestamp
	authorization += "/" + strconv.Itoa(option.ExpirationPeriodInSeconds)
	signature := sign(credentials, req, option)
	authorization += "/" + option.signedHeadersToString() + "/" + signature

	req.setHeader("Authorization", authorization)

	return authorization
}

// Client is the base client implemention for Baidu Cloud API.
type Client struct {
	*Config
	httpClient *http.Client
	debug      bool
}

func NewClient(config *Config) *Client {
	return &Client{config, newHttpClient(config), false}
}

// SetDebug enables debug mode of bce.Client instance.
func (c *Client) SetDebug(debug bool) {
	c.debug = debug
}

func newHttpClient(config *Config) *http.Client {
	transport := new(http.Transport)

	if defaultTransport, ok := http.DefaultTransport.(*http.Transport); ok {
		transport.Proxy = defaultTransport.Proxy
		transport.Dial = defaultTransport.Dial
		transport.TLSHandshakeTimeout = defaultTransport.TLSHandshakeTimeout
	}

	if config.ProxyHost != "" {
		host := config.ProxyHost

		if config.ProxyPort > 0 {
			host += ":" + strconv.Itoa(config.ProxyPort)
		}

		proxyUrl, err := url.Parse(util.HostToURL(host, "http"))

		if err != nil {
			panic(err)
		}

		transport.Proxy = http.ProxyURL(proxyUrl)
	}

	/*
		if c.ConnectionTimeout > 0 {
			transport.TLSHandshakeTimeout = c.ConnectionTimeout
		}
	*/

	if config.MaxConnections > 0 {
		transport.MaxIdleConnsPerHost = config.MaxConnections
	}

	return &http.Client{
		Transport: transport,
		Timeout:   config.Timeout,
	}
}

// GetURL generates the full URL of http request for Baidu Cloud API.
func (c *Client) GetURL(host, uriPath string, params map[string]string) string {
	if strings.Index(uriPath, "/") == 0 {
		uriPath = uriPath[1:]
	}

	if c.APIVersion != "" {
		uriPath = fmt.Sprintf("%s/%s", c.APIVersion, uriPath)
	}

	return util.GetURL(c.Protocol, host, uriPath, params)
}

// SessionTokenRequest contains all options for STS（Security Token Service）of Baidu Cloud API.
//
// For details, please refer https://cloud.baidu.com/doc/BOS/API.html#STS.E7.AE.80.E4.BB.8B
type SessionTokenRequest struct {
	DurationSeconds   int                     `json:"durationSeconds"`
	Id                string                  `json:"id"`
	AccessControlList []AccessControlListItem `json:"accessControlList"`
}

// AccessControlListItem contains sub options for bce.SessionTokenRequest
//
// For details, please refer https://cloud.baidu.com/doc/BOS/API.html#STS.E7.AE.80.E4.BB.8B
type AccessControlListItem struct {
	Eid        string   `json:"eid"`
	Service    string   `json:"service"`
	Region     string   `json:"region"`
	Effect     string   `json:"effect"`
	Resource   []string `json:"resource"`
	Permission []string `json:"permission"`
}

// SessionTokenResponse contains all response fields for STS（Security Token Service）of Baidu Cloud API.
//
// For details, please refer https://cloud.baidu.com/doc/BOS/API.html#STS.E7.AE.80.E4.BB.8B
type SessionTokenResponse struct {
	AccessKeyId     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	SessionToken    string `json:"sessionToken"`
	CreateTime      string `json:"createTime"`
	Expiration      string `json:"expiration"`
	UserId          string `json:"userId"`
}

// GetSessionToken gets response for STS（Security Token Service）of Baidu Cloud API.
//
// For details, please refer https://cloud.baidu.com/doc/BOS/API.html#STS.E7.AE.80.E4.BB.8B
func (c *Client) GetSessionToken(sessionTokenRequest SessionTokenRequest,
	option *SignOption) (*SessionTokenResponse, error) {

	var params map[string]string

	if sessionTokenRequest.DurationSeconds > 0 {
		params = map[string]string{"durationSeconds": strconv.Itoa(sessionTokenRequest.DurationSeconds)}
	}

	body, err := util.ToJson(sessionTokenRequest, "id", "accessControlList")

	if err != nil {
		return nil, err
	}

	uriPath := "sessionToken"

	if c.APIVersion == "" {
		uriPath = "v1/" + uriPath
	}

	req, err := NewRequest("POST", c.GetURL("sts.bj.baidubce.com", uriPath, params), bytes.NewReader(body))

	if err != nil {
		return nil, err
	}

	option = CheckSignOption(option)
	option.AddHeader("Content-Type", "application/json")

	resp, err := c.SendRequest(req, option)

	if err != nil {
		return nil, err
	}

	bodyContent, err := resp.GetBodyContent()

	if err != nil {
		return nil, err
	}

	var sessionTokenResponse *SessionTokenResponse
	err = json.Unmarshal(bodyContent, &sessionTokenResponse)

	if err != nil {
		return nil, err
	}

	return sessionTokenResponse, nil
}

// SendRequest sends a http request to the endpoint of Baidu Cloud API.
func (c *Client) SendRequest(req *Request, option *SignOption) (bceResponse *Response, err error) {
	if option == nil {
		option = &SignOption{}
	}

	option.AddHeader("User-Agent", c.GetUserAgent())

	if c.RetryPolicy == nil {
		c.RetryPolicy = NewDefaultRetryPolicy(3, 20*time.Second)
	}

	for i := 0; ; i++ {
		bceResponse, err = nil, nil

		if option.Credentials != nil {
			GenerateAuthorization(*option.Credentials, *req, option)
		} else {
			GenerateAuthorization(*c.Credentials, *req, option)
		}

		if c.debug {
			util.Debug("", fmt.Sprintf("Request: httpMethod = %s, requestUrl = %s, requestHeader = %v",
				req.Method, req.URL.String(), req.Header))
		}

		resp, httpError := c.httpClient.Do(req.raw())

		if c.debug {
			statusCode := -1

			if resp != nil {
				statusCode = resp.StatusCode
			}

			util.Debug("", fmt.Sprintf("Response: status code = %d, httpMethod = %s, requestUrl = %s",
				statusCode, req.Method, req.URL.String()))
		}

		if httpError != nil {
			duration := c.RetryPolicy.GetDelayBeforeNextRetry(httpError, i+1)

			if duration <= 0 {
				err = httpError
				return
			} else {
				time.Sleep(duration)
				continue
			}

		}

		bceResponse = NewResponse(resp)

		if resp.StatusCode >= http.StatusBadRequest {
			err = buildError(bceResponse)
		}

		if err == nil {
			return
		}

		duration := c.RetryPolicy.GetDelayBeforeNextRetry(err, i+1)

		if duration <= 0 {
			return
		}

		time.Sleep(duration)
	}
}

func generateHeaderValidCompareFunc(headerKey string) func(string, string) bool {
	return func(key, value string) bool {
		return strings.ToLower(key) == strings.ToLower(headerKey) && value != ""
	}
}

// sign returns signed signature.
func sign(credentials Credentials, req Request, option *SignOption) string {
	signingKey := getSigningKey(credentials, option)
	req.prepareHeaders(option)
	canonicalRequest := req.canonical(option)
	signature := util.HmacSha256Hex(signingKey, canonicalRequest)

	return signature
}

func getSigningKey(credentials Credentials, option *SignOption) string {
	var authStringPrefix = fmt.Sprintf("bce-auth-v1/%s", credentials.AccessKeyID)
	authStringPrefix += "/" + option.Timestamp
	authStringPrefix += "/" + strconv.Itoa(option.ExpirationPeriodInSeconds)

	return util.HmacSha256Hex(credentials.SecretAccessKey, authStringPrefix)
}
