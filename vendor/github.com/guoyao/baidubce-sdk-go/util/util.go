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
 * @file util.go
 * @author guoyao
 */

// Package util implements a set of util functions.
package util

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

func GetURL(protocol, host, uriPath string, params map[string]string) string {
	if strings.Index(uriPath, "/") == 0 {
		uriPath = uriPath[1:]
	}

	query := strings.Trim(ToCanonicalQueryString(params), " ")

	if query == "" {
		return fmt.Sprintf("%s/%s", HostToURL(host, protocol), uriPath)
	}

	return fmt.Sprintf("%s/%s?%s", HostToURL(host, protocol), uriPath, query)
}

// GetURIPath returns the path part of URI.
func GetURIPath(uri string) string {
	uri = strings.Replace(uri, "://", "", 1)
	index := strings.Index(uri, "/")
	return uri[index:]
}

// URIEncodeExceptSlash encodes all characters of a string except the slash character.
func URIEncodeExceptSlash(uri string) string {
	var result string

	for _, char := range uri {
		str := fmt.Sprintf("%c", char)
		if str == "/" {
			result += str
		} else {
			result += URLEncode(str)
		}
	}

	return result
}

// HmacSha256Hex returns a encrypted string.
func HmacSha256Hex(key, message string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func GetMD5(data interface{}, base64Encode bool) string {
	hash := md5.New()

	if str, ok := data.(string); ok {
		io.Copy(hash, strings.NewReader(str))
	} else if byteArray, ok := data.([]byte); ok {
		io.Copy(hash, bytes.NewReader(byteArray))
	} else if reader, ok := data.(io.Reader); ok {
		if f, ok := data.(io.Seeker); ok {
			f.Seek(0, 0)
			io.Copy(hash, reader)
			f.Seek(0, 0)
		} else {
			io.Copy(hash, reader)
		}
	} else {
		panic("data type should be string or []byte or io.Reader.")
	}

	if base64Encode {
		return Base64Encode(hash.Sum(nil))
	}

	return hex.EncodeToString(hash.Sum(nil))
}

func GetSha256(data interface{}) string {
	hash := sha256.New()

	if str, ok := data.(string); ok {
		io.Copy(hash, strings.NewReader(str))
	} else if byteArray, ok := data.([]byte); ok {
		io.Copy(hash, bytes.NewReader(byteArray))
	} else if reader, ok := data.(io.Reader); ok {
		if f, ok := data.(io.Seeker); ok {
			f.Seek(0, 0)
			io.Copy(hash, reader)
			f.Seek(0, 0)
		} else {
			io.Copy(hash, reader)
		}
	} else {
		panic("data type should be string or []byte or io.Reader.")
	}

	return hex.EncodeToString(hash.Sum(nil))
}

func Base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// Contains determines whether a string slice contains a certain value.
// Ignore case when comparing if case insensitive.
func Contains(slice []string, value string, caseInsensitive bool) bool {
	if caseInsensitive {
		value = strings.ToLower(value)
	}

	for _, v := range slice {
		if caseInsensitive {
			v = strings.ToLower(v)
		}

		if value == v {
			return true
		}
	}

	return false
}

// MapContains determines whether the string map contains a uncertain value.
// The result is determined by compare function.
func MapContains(m map[string]string, compareFunc func(string, string) bool) bool {
	for key, value := range m {
		if compareFunc(key, value) {
			return true
		}
	}

	return false
}

// GetMapKey returns the key of the map for a certain value.
// Ignore case when comparing if case insensitive.
func GetMapKey(m map[string]string, key string, caseInsensitive bool) string {
	if caseInsensitive {
		key = strings.ToLower(key)
	}

	var tempKey string

	for k := range m {
		tempKey = k

		if caseInsensitive {
			tempKey = strings.ToLower(k)
		}

		if tempKey == key {
			return k
		}
	}

	return ""
}

// GetMapValue returns the value of the map for a certain key.
// Ignore case when comparing if case insensitive.
func GetMapValue(m map[string]string, key string, caseInsensitive bool) string {
	if caseInsensitive {
		for k, v := range m {
			if strings.ToLower(k) == strings.ToLower(key) {
				return v
			}
		}
	}

	return m[key]
}

// TimeToUTCString returns a utc string of a time instance.
func TimeToUTCString(t time.Time) string {
	format := time.RFC3339 // 2006-01-02T15:04:05Z07:00
	return t.UTC().Format(format)
}

// TimeStringToRFC1123 returns a formatted string of `time.RFC1123` format.
func TimeStringToRFC1123(str string) string {
	t, err := time.Parse(time.RFC3339, str)
	if err != nil {
		t, err = time.Parse(time.RFC1123, str)
		if err != nil {
			panic("Time format invalid. The time format must be time.RFC3339 or time.RFC1123")
		}
	}

	return t.Format(time.RFC1123)
}

// HostToURL returns the whole URL string.
func HostToURL(host, protocol string) string {
	if matched, _ := regexp.MatchString("^[[:alpha:]]+:", host); matched {
		return host
	}

	if protocol == "" {
		protocol = "http"
	}

	return fmt.Sprintf("%s://%s", protocol, host)
}

// ToCanonicalQueryString returns the canonicalized query string.
func ToCanonicalQueryString(params map[string]string) string {
	if params == nil {
		return ""
	}

	encodedQueryStrings := make([]string, 0, 10)
	var query string

	for key, value := range params {
		if key != "" {
			query = URLEncode(key) + "="
			if value != "" {
				query += URLEncode(value)
			}
			encodedQueryStrings = append(encodedQueryStrings, query)
		}
	}

	sort.Strings(encodedQueryStrings)

	return strings.Join(encodedQueryStrings, "&")
}

// ToCanonicalHeaderString returns the canonicalized string.
func ToCanonicalHeaderString(headerMap map[string]string) string {
	headers := make([]string, 0, len(headerMap))
	for key, value := range headerMap {
		headers = append(headers,
			fmt.Sprintf("%s:%s", URLEncode(strings.ToLower(key)),
				URLEncode(strings.TrimSpace(value))))
	}

	sort.Strings(headers)

	return strings.Join(headers, "\n")
}

// URLEncode encodes a string like Javascript's encodeURIComponent()
func URLEncode(str string) string {
	// BUG(go): see https://github.com/golang/go/issues/4013
	// use %20 instead of the + character for encoding a space
	return strings.Replace(url.QueryEscape(str), "+", "%20", -1)
}

// SliceToLower transforms each item of a slice to lowercase.
func SliceToLower(slice []string) {
	for index, value := range slice {
		slice[index] = strings.ToLower(value)
	}
}

// MapKeyToLower transforms each item of a map to lowercase.
func MapKeyToLower(m map[string]string) {
	temp := make(map[string]string, len(m))
	for key, value := range m {
		temp[strings.ToLower(key)] = value
		delete(m, key)
	}
	for key, value := range temp {
		m[key] = value
	}
}

// Convert anything to map
func ToMap(i interface{}, keys ...string) (map[string]interface{}, error) {
	var m map[string]interface{}
	var byteArray []byte

	if str, ok := i.(string); ok {
		byteArray = []byte(str)
	} else if b, ok := i.([]byte); ok {
		byteArray = b
	} else {
		b, err := json.Marshal(i)

		if err != nil {
			return nil, err
		}

		byteArray = b
	}

	if err := json.Unmarshal(byteArray, &m); err != nil {
		return nil, err
	}

	if keys != nil && len(keys) > 0 {
		result := make(map[string]interface{}, len(keys))

		for _, k := range keys {
			if v, ok := m[k]; ok {
				result[k] = v
			}
		}

		return result, nil
	}

	return m, nil
}

func ToJson(i interface{}, keys ...string) ([]byte, error) {
	byteArray, err := json.Marshal(i)

	if keys == nil || len(keys) == 0 {
		return byteArray, err
	}

	if err == nil {
		m, err := ToMap(byteArray, keys...)

		if err != nil {
			return nil, err
		}

		byteArray, err = json.Marshal(m)
	}

	return byteArray, err
}

func CheckFileExists(filename string) bool {
	exist := true

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		exist = false
	}

	return exist
}

func TempFileWithSize(fileSize int64) (*os.File, error) {
	f, err := TempFile(nil, "", "")

	if err != nil {
		return nil, err
	}

	if err = f.Truncate(fileSize); err != nil {
		return nil, err
	}

	return f, nil
}

func TempFile(content []byte, dir, prefix string) (*os.File, error) {
	if dir == "" {
		home, err := HomeDir()

		if err != nil {
			return nil, err
		}

		dir = path.Join(home, "tmp")
	}

	if prefix == "" {
		prefix = "temp"
	}

	if !CheckFileExists(dir) {
		err := os.MkdirAll(dir, 0744)

		if err != nil {
			return nil, err
		}
	}

	tmpfile, err := ioutil.TempFile(dir, prefix)

	if err != nil {
		return nil, err
	}

	if content != nil {
		if _, err := tmpfile.Write(content); err != nil {
			return nil, err
		}
	}

	_, err = tmpfile.Seek(0, 0)

	if err != nil {
		return nil, err
	}

	return tmpfile, nil
}

var homeDir string

func HomeDir() (string, error) {
	if homeDir != "" {
		return homeDir, nil
	}

	var result string
	var err error

	if runtime.GOOS == "windows" {
		result, err = dirWindows()
	} else {
		result, err = dirUnix()
	}

	if err != nil {
		return "", err
	}

	homeDir = result

	return result, nil
}

func dirUnix() (string, error) {
	// First prefer the HOME environmental variable
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}

	// If that fails, try getent
	var stdout bytes.Buffer
	cmd := exec.Command("getent", "passwd", strconv.Itoa(os.Getuid()))
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		// If "getent" is missing, ignore it
		if err == exec.ErrNotFound {
			return "", err
		}
	} else {
		if passwd := strings.TrimSpace(stdout.String()); passwd != "" {
			// username:password:uid:gid:gecos:home:shell
			passwdParts := strings.SplitN(passwd, ":", 7)

			if len(passwdParts) > 5 {
				return passwdParts[5], nil
			}
		}
	}

	// If all else fails, try the shell
	stdout.Reset()
	cmd = exec.Command("sh", "-c", "cd && pwd")
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", err
	}

	result := strings.TrimSpace(stdout.String())

	if result == "" {
		return "", errors.New("blank output when reading home directory")
	}

	return result, nil
}

func dirWindows() (string, error) {
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}

	drive := os.Getenv("HOMEDRIVE")
	path := os.Getenv("HOMEPATH")
	home := drive + path

	if drive == "" || path == "" {
		home = os.Getenv("USERPROFILE")
	}

	if home == "" {
		return "", errors.New("HOMEDRIVE, HOMEPATH, and USERPROFILE are blank")
	}

	return home, nil
}

func Debug(title, message string) {
	if title != "" {
		log.Println("----------------------------DEBUG: start of " + title + "----------------------------")
	}

	log.Println(message)

	if title != "" {
		log.Println("----------------------------DEBUG: end of " + title + "------------------------------\n")
	}
}

// FormatTest returns a formatted string.
func FormatTest(funcName, got, expected string) string {
	return fmt.Sprintf("%s failed. Got %s, expected %s", funcName, got, expected)
}
