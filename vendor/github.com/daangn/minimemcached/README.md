# Minimemcached


[![Go Reference](https://pkg.go.dev/badge/github.com/daangn/minimemcached.svg)](https://pkg.go.dev/github.com/daangn/minimemcached)
[![Test](https://github.com/daangn/minimemcached/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/daangn/minimemcached/actions/workflows/test.yml)

Minimemcached is a Memcached server for written in Go, aimed for unittests in Go projects.

---

When you have to test codes that use Memcached server, running actual Memcached server instance could be quite expensive,
depending on your environment.

Minimemcached aims to solve this problem by implementing Memcached's TCP interface 100% in Go,
and works perfectly well with [gomemcache](https://github.com/bradfitz/gomemcache), a memcache client for Go.

<details><summary>Implemented commands</summary>

<p>

- get
- gets
- cas
- set
- touch
- add
- replace
- append
- prepend
- delete
- incr
- decr
- flush_all
- version

</p>
</details>

## Setup

- To use Minimemcached, you can import it as below.
  You can also view [example code](https://github.com/daangn/minimemcached/blob/main/examples/main.go).

```go
package main

import (
	"fmt"

	"github.com/bradfitz/gomemcache/memcache"

	"github.com/daangn/minimemcached"
)

func main() {
	cfg := &minimemcached.Config{
		Port: 8080,
	}
	m, err := minimemcached.Run(cfg)
	if err != nil {
		fmt.Println("failed to start mini-memcached server.")
		return
	}

	defer m.Close()

	fmt.Println("mini-memcached started")

	mc := memcache.New(fmt.Sprintf("localhost:%d", m.Port()))
	err = mc.Set(&memcache.Item{Key: "foo", Value: []byte("my value"), Expiration: int32(60)})
	if err != nil {
		fmt.Printf("err(set): %v\n", err)
	}

	it, err := mc.Get("foo")
	if err != nil {
		fmt.Printf("err(get): %v\n", err)
	} else {
		fmt.Printf("key: %s, value: %s\n", it.Key, it.Value)
		fmt.Printf("value: %s\n", string(it.Value))
	}
}

```

## Benchmarks

- Running same test cases on memcached server on a docker and minimemcached, minimemcached outperformed memcached running on docker container.
- You can run this benchmark yourself. Check [here](https://github.com/daangn/minimemcached/tree/main/benchmarks).

<details><summary>Benchmark Environment</summary>
<p>

* goos: darwin
* goarch: arm64
* memcached docker image: `memcached:1.5.16`

</p>
</details>

### Results

```
# Memcached running on a docker container.
BenchmarkMemcached-8       	     610	   1721643 ns/op
BenchmarkMemcached-8       	     729	   1714716 ns/op
BenchmarkMemcached-8       	     717	   1716914 ns/op
BenchmarkMemcached-8       	     698	   1783312 ns/op
BenchmarkMemcached-8       	     693	   1784781 ns/op

# Minimemcached.
BenchmarkMinimemcached-8    24710	     46661 ns/op
BenchmarkMinimemcached-8    24684	     47918 ns/op
BenchmarkMinimemcached-8    24866	     47558 ns/op
BenchmarkMinimemcached-8    25046	     46770 ns/op
BenchmarkMinimemcached-8    26085	     46707 ns/op
```

> `op`: set, get, delete operation

- As shown in the result above, minimemcached took about 47122.8 ns per operation, when memcached took about 1744273.2 ns per operation.


## Author

- [@sang-w0o](https://github.com/sang-w0o)
- [@mingrammer](https://github.com/mingrammer)
- [@HurSungYun](https://github.com/HurSungYun)
- [@MeteorSis](https://github.com/MeteorSis)
- [@JungleKim](https://github.com/JungleKim)
- [@novemberde](https://github.com/novemberde)

## Contributions

- If you want to contribute to Minimemcached, feel free to create an issue or pull request!
