package util

import (
	"bytes"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

//CreateRandomString create random string
func CreateRandomString() string {

	rand.Seed(time.Now().UnixNano())
	randInt := rand.Int63()
	randStr := strconv.FormatInt(randInt, 36)

	return randStr
}

// Encode encodes the values into ``URL encoded'' form
// ("acl&bar=baz&foo=quux") sorted by key.
func Encode(v url.Values) string {
	if v == nil {
		return ""
	}
	var buf bytes.Buffer
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		vs := v[k]
		prefix := url.QueryEscape(k)
		for _, v := range vs {
			if buf.Len() > 0 {
				buf.WriteByte('&')
			}
			buf.WriteString(prefix)
			if v != "" {
				buf.WriteString("=")
				buf.WriteString(url.QueryEscape(v))
			}
		}
	}
	return buf.String()
}

func GetGMTime() string {
	return time.Now().UTC().Format(http.TimeFormat)
}
