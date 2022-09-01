
# UCloud 对象存储 SDK <a href="https://godoc.org/github.com/ufilesdk-dev/ufile-gosdk"><img src="https://godoc.org/github.com/ufilesdk-dev/ufile-gosdk?status.svg" alt="GoDoc"></a>
> Modules are interface and implementation.  
> The best modules are where interface is much simpler than implementation.  
> **By: John Ousterhout**

Table of Contents
=================

* [概述](#概述)
	* [US3 对象存储基本概念](#US3%20对象存储基本概念)
	* [签名](#签名)
* [快速使用](#快速使用)
    * [下载安装](#下载安装)
		* [环境要求](#环境要求)
      	* [SDK下载](#SDK下载)
      	* [配置文件](#配置文件)
    * [执行测试](#执行测试)
* [示例代码](#示例代码)
	* [存储空间管理](#存储空间管理)
	* [对象/文件管理](#对象/文件管理)
        * [普通上传](#普通上传)
        * [表单上传](#表单上传)
        * [秒传](#秒传)
		* [流式上传](#流式上传)
        * [分片上传](#分片上传)
		* [上传回调](#上传回调)
        * [文件下载](#文件下载)
        * [查询文件基本信息](#查询文件基本信息)
        * [删除文件](#删除文件)
        * [文件解冻](#文件解冻)
        * [文件存储类型转换](#文件存储类型转换)
        * [比较本地文件和远程文件etag](#比较本地文件和远程文件etag)
        * [文件拷贝](#文件拷贝)
        * [文件重命名](#文件重命名)
		* [前缀列表查询](#前缀列表查询)
        * [获取目录文件列表](#获取目录文件列表)
* [文档说明](#文档说明)
* [联系我们](#联系我们)

# 概述

## US3 对象存储基本概念

在对象存储系统中，存储空间（Bucket）是文件（File）的组织管理单位，文件（File）是存储空间的逻辑存储单元。对于每个账号，该账号里存放的每个文件都有唯一的一对存储空间（Bucket）与键（Key）作为标识。我们可以把 Bucket 理解成一类文件的集合，Key 理解成文件名。由于每个 Bucket 需要配置和权限不同，每个账户里面会有多个 Bucket。在 US3 里面，Bucket 主要分为公有和私有两种，公有 Bucket 里面的文件可以对任何人开放，私有 Bucket 需要配置对应访问签名才能访问。

## 签名 

本 SDK 接口是基于 HTTP 的，为了连接的安全性，US3 使用 HMAC SHA1 对每个连接进行签名校验。使用本 SDK 可以忽略签名相关的算法过程，只要把公私钥写入到配置文件里面，读取并传给 UFileRequest 里面的 New 方法即可。签名相关的算法与详细实现请见 [Auth 模块](https://github.com/ufilesdk-dev/ufile-gosdk/blob/master/auth.go)

# 快速使用

## 下载安装

### 环境要求

- Golanng 版本 (待校验)

### SDK下载

- 快速安装：go get [github.com/ufilesdk-dev/ufile-gosdk](http://github.com/ufilesdk-dev/ufile-gosdk)
- [历史版本下载](https://github.com/ufilesdk-dev/ufile-gosdk/releases)

### 配置文件

- 进入目录 [github.com/ufilesdk-dev/ufile-gosdk](http://github.com/ufilesdk-dev/ufile-gosdk) 下，按说明填写 config.json

```JSON
{
    "说明1":"管理 bucket 创建和删除必须要公私钥(见 https://console.ucloud.cn/uapi/apikey)，如果只做文件上传和下载用 TOEKN (见 https://console.ucloud.cn/ufile/token)就够了，为了安全，强烈建议只使用 TOKEN 做文件管理",
    "public_key":"",
    "private_key":"",

    "说明2":"以下两个参数是用来管理文件用的。对应的是 file.go 里面的接口，file_host 是不带 bucket 名字的。比如：北京地域的host填cn-bj.ufileos.com，而不是填 bucketname.cn-bj.ufileos.com。若为自定义域名，请直接带上 http 开头的 URL。如：http://example.com",
    "bucket_name":"",
    "file_host":"",

    "说明3":"verifyUploadMD5 用于数据完整性校验，默认不开启，若要开启请置为true",
    "verifyUploadMD5": false
}
```

> 密钥可以在控制台中 [API 产品 - API 密钥](https://console.ucloud.cn/uapi/apikey)，点击显示 API 密钥获取。将 public_key 和 private_key 分别赋值给相关变量后，SDK即可通过此密钥完成鉴权。请妥善保管好 API 密钥，避免泄露。
> token（令牌）是针对指定bucket授权的一对公私钥。可通过token进行授权bucket的权限控制和管理。可以在控制台中[对象存储US3-令牌管理](https://console.ucloud.cn/ufile/token)，点击创建令牌获取。

### 运行demo

- 目录 [github.com/ufilesdk-dev/ufile-gosdk](http://github.com/ufilesdk-dev/ufile-gosdk) 下执行`go run demo.go`

### 导入使用

在您的项目代码中，使用`import ufsdk "github.com/ufilesdk-dev/ufile-gosdk"`引入US3 Go SDK的包

# 示例代码

## 存储空间管理

### 创建存储空间

```go
// 加载配置，创建请求
config, err := ufsdk.LoadConfig("config.json")
if err != nil {
	panic(err.Error())
}
req, err := ufsdk.NewBucketRequest(config, nil)
if err != nil {
	panic(err.Error())
}

bucketRet, err := req.CreateBucket("BucketName", "Region", "BucketType", "ProjectId")
if err != nil {
	log.Fataf("创建 bucket 出错，错误信息为：%s\n", err.Error())
}
```

[回到目录](#table-of-contents)

### 获取存储空间信息

```go
// 加载配置，创建请求
config, err := ufsdk.LoadConfig("config.json")
if err != nil {
	panic(err.Error())
}
req, err := ufsdk.NewBucketRequest(config, nil)
if err != nil {
	panic(err.Error())
}

bucketList, err := req.DescribeBucket("BucketName", Offset, Limit, "ProjectId")
if err != nil {
	log.Println("获取 bucket 信息出错，错误信息为：", err.Error())
}
```

[回到目录](#table-of-contents)

### 更新存储空间访问类型


```go
// 加载配置，创建请求
config, err := ufsdk.LoadConfig("config.json")
if err != nil {
	panic(err.Error())
}
req, err := ufsdk.NewBucketRequest(config, nil)
if err != nil {
	panic(err.Error())
}

bucketRet, err = req.UpdateBucket("BucketName", "BucketType", "ProjectId")
if err != nil {
	log.Println("更新 bucket 信息失败，错误信息为：", err.Error())
}
```

[回到目录](#table-of-contents)

### 删除存储空间

```go
// 加载配置，创建请求
config, err := ufsdk.LoadConfig("config.json")
if err != nil {
	panic(err.Error())
}
req, err := ufsdk.NewBucketRequest(config, nil)
if err != nil {
	panic(err.Error())
}

bucketRet, err = req.DeleteBucket("BucketName", "ProjectId")
if err != nil {
	log.Fataf("删除 bucket 失败，错误信息为：", err.Error())
}
```

[回到目录](#table-of-contents)

<a name="对象/文件管理"></a>
## 对象/文件管理

<a name="普通上传"></a>
### 普通上传

- demo程序

```go
// 加载配置，创建请求
config, err := ufsdk.LoadConfig("config.json")
if err != nil {
	panic(err.Error())
}
req, err := ufsdk.NewFileRequest(config, nil)
if err != nil {
	panic(err.Error())
}

err = req.PutFile("FilePath", "KeyName", "MimeType")
if err != nil {
	log.Println("DumpResponse：", string(req.DumpResponse(true)))
}
```

[回到目录](#table-of-contents)

<a name="表单上传"></a>
### 表单上传

- demo程序

```go
// 加载配置，创建请求
config, err := ufsdk.LoadConfig("config.json")
if err != nil {
	panic(err.Error())
}
req, err := ufsdk.NewFileRequest(config, nil)
if err != nil {
	panic(err.Error())
}

err = req.PostFile("FilePath", "KeyName", "MimeType")
if err != nil {
	log.Println("DumpResponse：", string(req.DumpResponse(true)))
}
```

[回到目录](#table-of-contents)

<a name="秒传"></a>
### 秒传

- demo程序

```go
// 加载配置，创建请求
config, err := ufsdk.LoadConfig("config.json")
if err != nil {
	panic(err.Error())
}
req, err := ufsdk.NewFileRequest(config, nil)
if err != nil {
	panic(err.Error())
}

err = req.UploadHit("FilePath", "KeyName")
if err != nil {
	log.Println("DumpResponse：", string(req.DumpResponse(true)))
}
```

[回到目录](#table-of-contents)

<a name="流式上传"></a>
### 流式上传

- demo程序

```go
if err != nil {
    log.Fatal(err.Error())
}
req, err := ufsdk.NewFileRequest(config, nil)
if err != nil {
    log.Fatal(err.Error())
}
// 流式上传本地小文件
f, err := os.Open("FilePath")
if err != nil {
    panic(err.Error())
}
err = req.IOPut(f, "KeyName", "")
f.Close()
if err != nil {
    log.Fatalf("%s\n", req.DumpResponse(true))
}

// 流式上传大文件
f1, err := os.Open("FilePath1")
if err != nil {
    panic(err.Error())
}
err = req.IOMutipartAsyncUpload(f1, "KeyName", "")
f1.Close()
if err != nil {
    log.Fatalf("%s\n", req.DumpResponse(true))
}
```

[回到目录](#table-of-contents)


<a name="分片上传"></a>
### 分片上传

- demo程序

```go
// 加载配置，创建请求
config, err := ufsdk.LoadConfig("config.json")
if err != nil {
	panic(err.Error())
}
req, err := ufsdk.NewFileRequest(config, nil)
if err != nil {
	panic(err.Error())
}

err = req.MPut("FilePath", "KeyName", "MimeType")
if err != nil {
	log.Println("DumpResponse：", string(req.DumpResponse(true)))
}
```

[回到目录](#table-of-contents)

<a name="上传回调"></a>
### 上传回调

- demo程序

```go
// 加载配置，创建请求
config, err := ufsdk.LoadConfig("config.json")
if err != nil {
	panic(err.Error())
}
req, err := ufsdk.NewFileRequest(config, nil)
if err != nil {
	panic(err.Error())
}

// 同步分片上传回调
err = req.MPutWithPolicy("FilePath", "KeyName", "MimeType", "Policy")
if err != nil {
	log.Println("DumpResponse：", string(req.DumpResponse(true)))
}

// 异步分片上传回调
err = req.AsyncMPutWithPolicy("FilePath", "KeyName", "MimeType", "Policy")
if err != nil {
	log.Println("DumpResponse：", string(req.DumpResponse(true)))
}

// 异步分片并发上传回调
jobs := 20 // 并发数为 20
err = req.AsyncUploadWithPolicy("FilePath", "KeyName", "MimeType", jobs, "Policy")
if err != nil {
	log.Println("DumpResponse：", string(req.DumpResponse(true)))
}
```

[回到目录](#table-of-contents)

<a name="文件下载"></a>
### 文件下载

- demo程序

```go
// 加载配置，创建请求
config, err := ufsdk.LoadConfig("config.json")
if err != nil {
	panic(err.Error())
}
req, err := ufsdk.NewFileRequest(config, nil)
if err != nil {
	panic(err.Error())
}
// 普通下载
err = req.Download("DownLoadURL")
if err != nil {
	log.Println("DumpResponse：", string(req.DumpResponse(true)))
}
// 流式下载
err = req.Download("buffer", "KeyName")
if err != nil {
	log.Println("DumpResponse：", string(req.DumpResponse(true)))
}
```

[回到目录](#table-of-contents)


<a name="查询文件基本信息"></a>
### 查询文件基本信息

- demo程序

```go
config, err := ufsdk.LoadConfig("config.json")
if err != nil {
	panic(err.Error())
}
req, err := ufsdk.NewFileRequest(config, nil)
if err != nil {
	panic(err.Error())
}
err = req.HeadFile("KeyName")
if err != nil {
	log.Println("DumpResponse：", string(req.DumpResponse(true)))
}
```

[回到目录](#table-of-contents)


<a name="删除文件"></a>
### 删除文件

- demo程序

```go
config, err := ufsdk.LoadConfig("config.json")
if err != nil {
	panic(err.Error())
}
req, err := ufsdk.NewFileRequest(config, nil)
if err != nil {
	panic(err.Error())
}
err = req.DeleteFile("KeyName")
if err != nil {
	log.Println("DumpResponse：", string(req.DumpResponse(true)))
}
```

[回到目录](#table-of-contents)


<a name="文件解冻"></a>
### 文件解冻

- demo 程序

```go
config, err := ufsdk.LoadConfig("config.json")
if err != nil {
	panic(err.Error())
}
req, err := ufsdk.NewFileRequest(config, nil)
if err != nil {
	panic(err.Error())
}
err = req.Restore("KeyName")
if err != nil {
	log.Println("DumpResponse：", string(req.DumpResponse(true)))
}
```

[回到目录](#table-of-contents)


<a name="文件存储类型转换"></a>
### 文件存储类型转换

- demo 程序

```go
config, err := ufsdk.LoadConfig("config.json")
if err != nil {
	panic(err.Error())
}
req, err := ufsdk.NewFileRequest(config, nil)
if err != nil {
	panic(err.Error())
}
err = req.ClassSwitch("KeyName", "StorageClass")
if err != nil {
	log.Println("DumpResponse：", string(req.DumpResponse(true)))
}
```

[回到目录](#table-of-contents)


<a name="比较本地文件和远程文件etag"></a>
### 比较本地文件和远程文件etag

- demo程序

```go
config, err := ufsdk.LoadConfig("config.json")
if err != nil {
	panic(err.Error())
}
req, err := ufsdk.NewFileRequest(config, nil)
if err != nil {
	panic(err.Error())
}
err = req.CompareFileEtag("KeyName", "FilePath")
if err != nil {
	log.Println("DumpResponse：", string(req.DumpResponse(true)))
}
```

[回到目录](#table-of-contents)


<a name="文件拷贝"></a>
### 文件拷贝

- demo程序

```go
config, err := ufsdk.LoadConfig("config.json")
if err != nil {
	panic(err.Error())
}
req, err := ufsdk.NewFileRequest(config, nil)
if err != nil {
	panic(err.Error())
}
err = req.Copy("DstkeyName", "SrcBucketName", "SrcKeyName")
if err != nil {
	log.Println("DumpResponse：", string(req.DumpResponse(true)))
}
```

[回到目录](#table-of-contents)


<a name="文件重命名"></a>
### 文件重命名

- demo程序

```go
config, err := ufsdk.LoadConfig("config.json")
if err != nil {
	panic(err.Error())
}
req, err := ufsdk.NewFileRequest(config, nil)
if err != nil {
	panic(err.Error())
}
force := true // 为true则强制重命名
err = req.Rename("KeyName", "KeyName", force)
if err != nil {
	log.Println("DumpResponse：", string(req.DumpResponse(true)))
}
```

[回到目录](#table-of-contents)


<a name="前缀列表查询"></a>
### 前缀列表查询

- demo 程序

```go
config, err := ufsdk.LoadConfig("config.json")
if err != nil {
	panic(err.Error())
}
req, err := ufsdk.NewFileRequest(config, nil)
if err != nil {
	panic(err.Error())
}
list, err := req.PrefixFileList("Prefix", "Marker", "Limit")
if err != nil {
	log.Println("DumpResponse：", string(req.DumpResponse(true)))
}
```

[回到目录](#table-of-contents)


<a name="获取目录文件列表"></a>
### 获取目录文件列表

- demo 程序

```go
config, err := ufsdk.LoadConfig("config.json")
if err != nil {
	panic(err.Error())
}
req, err := ufsdk.NewFileRequest(config, nil)
if err != nil {
	panic(err.Error())
}
list, err := req.ListObjects("Prefix", "Marker", "Delimiter", "MaxKeys")
if err != nil {
	log.Println("DumpResponse：", string(req.DumpResponse(true)))
}
```

[回到目录](#table-of-contents)

# 文档说明

本 SDK 使用 [godoc](https://blog.golang.org/godoc-documenting-go-code) 约定的方法对每个 export 出来的接口进行注释。 你可以直接访问生成好的[在线文档](https://godoc.org/github.com/ufilesdk-dev/ufile-gosdk)。



# 联系我们

> - UCloud US3 [官方网站](https://www.ucloud.cn/site/product/ufile.html)
> - UCloud US3 [官方文档中心](https://docs.ucloud.cn/ufile/README)
> - UCloud 官方技术支持：[提交工单](https://accountv2.ucloud.cn/work_ticket/create)
