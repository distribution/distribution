# GO SDK 文档

## 简介

本文档主要介绍BOS GO SDK的安装和使用。在使用本文档前，您需要先了解
BOS的一些基本知识，并已开通了BOS服务。若您还不了解BOS，可以参考[产
品描述](https://cloud.baidu.com/doc/BOS/ProductDescription.html)和
[入门指南](https://cloud.baidu.com/doc/BOS/GettingStarted-new.html)。

## 安装SDK工具包

### 运行环境

GO SDK可以在go1.3及以上环境下运行。

### 安装步骤

 - 使用`go get`工具下载：`go get github.com/baidubce/bce-sdk-go`

### SDK目录结构

```
|--auth                   //BCE签名和权限认证
|--bce                    //BCE公用基础组件
|--http                   //http请求模块
|--services               //BCE相关服务目录
|  |--bos                 //BOS服务目录
|  |  |--bos_client.go    //BOS客户端入口
|  |  |--api              //BOS相关API目录
|  |     |--bucket.go     //BOS的Bucket相关API实现
|  |     |--object.go     //BOS的Object相关API实现
|  |     |--multipart.go  //BOS的Multipart相关API实现
|  |     |--module.go     //BOS相关API的数据模型
|  |     |--util.go       //BOS相关API实现使用的工具
|  |--sts                 //STS服务目录
|--util                   //BCE公用的工具实现
```

## 快速入门

1. 初始化一个BOS Client对象。
   BOS Client对象是与BOS服务交互的客户端，BOS GO SDK的BOS操作都是通过Client
   对象完成。用户可以参考`BosClient`节，完成初始化客户端的操作。
2. 新建一个Bucket。
   Bucket是BOS上的命名空间，相当于数据的容器，可以存储若干数据实体（Object）。
   用户可以参考新建`Bucket`来完成新建一个Bucket的操作。针对Bucket的命名规
   范，可以参考Bucket命名规范。
3. 上传Ojbect。
   Object是BOS中最基本的数据单元，用户可以把Object简单的理解为文件。用户
   可以参考最简单的上传完成对Object的上传。
4. 查看Object列表。
   当用户完成一系列上传后，可以参考查看Bucket中Object列表来查看指定Bucket
   下的全部Object。
5. 获取指定Object。
   用户可以参考简单的读取Object来实现对一个或者多个Object的获取。

## BOS Client

BOS Client对象是BOS服务的客户端，为开发者提供与BOS进行交互的所有接口。

### 新建BOS Client对象

#### 通过AK/SK方式访问BOS

用户使用AK/SK方式访问BOS时，可以参考如下代码创建一个BOS Client对象：

```
import (
    "github.com/baidubce/bce-sdk-go/services/bos"
)

func main() {
    AK, SK := <your-access-key-id>, <your-secret-access-key>
    ENDPOINT := "http://gz.bcebos.com"
    client, err := bos.NewClient(AK, SK, ENDPOINT)
}
```

上述代码中，变量`AK`、`SK`是由系统分配给用户的，均为字符串，用于标识
用户，为访问BOS做签名验证。其中`AK`对应控制台中的“Access Key ID”，`SK`
对应控制台中的“Access Key Secret”，获取方式请参考《操作指南管理ACCESSKEY》。
`ENDPOINT`是连接BOS服务的域名，上述指定为广州区域，用户可传入空字符串使
用默认域名`http://bj.bcebos.com`。其他配置见下文`配置BOS Client`一节。

> 注意：`ENDPOINT`参数只能用指定的包含区域的域名来进行定义，不指定时默
> 认为北京区域`http://bj.bcebos.com`。百度云目前开放了多区域支持，请参考
> 区域选择说明。目前支持“华北-北京”、“华南-广州”和“华东-苏州”三个区域。
> 北京区域：`http://bj.bcebos.com`，广州区域：`http://gz.bcebos.com`，
> 苏州区域：`http://su.bcebos.com`。

#### 通过STS方式访问BOS

用户可以通过STS方式访问BOS服务，详情见`STS签名认证`相关介绍。GO SDK提供
了STS服务用于申请临时认证字符串，然后将此认证字符串配置的到Client对象
中即可访问，具体参考如下代码：

```
import (
    "fmt"

    "github.com/baidubce/bce-sdk-go/auth"
    "github.com/baidubce/bce-sdk-go/services/bos"
    "github.com/baidubce/bce-sdk-go/services/sts"
)

func main() {
    // 创建STS服务的Client对象
    AK, SK := <your-access-key-id>, <your-secret-access-key>
    stsClient, err := sts.NewClient(AK, SK)
    if err != nil {
        fmt.Println("create sts client object :", err)
        return
    }

    // 获取临时认证token，有效期为60秒，ACL为空
    sts, err := stsClient.GetSessionToken(60, "")
    if err != nil {
        fmt.Println("get session token failed:", err)
        return
    }
    fmt.Println("GetSessionToken result:")
    fmt.Println("  accessKeyId:", sts.AccessKeyId)
    fmt.Println("  secretAccessKey:", sts.SecretAccessKey)
    fmt.Println("  sessionToken:", sts.SessionToken)
    fmt.Println("  createTime:", sts.CreateTime)
    fmt.Println("  expiration:", sts.Expiration)
    fmt.Println("  userId:", sts.UserId)

    // 使用STS的结果创建BOS服务的Client对象
    bosClient, err := bos.NewClient(sts.AccessKeyId, sts.SecretAccessKey, "")
    if err != nil {
        fmt.Println("create bos client failed:", err)
        return
    }
    stsCredential, err := auth.NewSessionBceCredentials(
            sts.AccessKeyId,
            sts.SecretAccessKey,
            sts.SessionToken)
    if err != nil {
        fmt.Println("create sts credential object failed:", err)
        return
    }
    bosClient.Config.Credentials = stsCredential
}
```

### 接口说明

BOS基于RESTful协议的接口对外提供服务，所有接口以官网API文档为依据实现于
`services/bos/api`目录下，分为`Bucket`相关接口、`Object`相关接
口和`Multipart`相关接口三部分。每个接口的参数分为必需参数和可选参数两
类，必需参数直接作为API函数的参数，可选参数以后缀名为`Args`的`struct`的
形式定义于`services/bos/api/model.go`文件中，如`CopyObject`接口的必需参数为`bucket名称`、
`object名称`和`CopySource`，但同时提供可选参数`CopyObjectArgs`，分别可以
用来指定拷贝的相关选项。 每个API函数返回的值也不同，分别以后缀名为`Result`
的`struct`定义于`model.go`文件中，具体某个API的返回参数的字段详见API说明
文档，`CopyObject`的参数和返回值定义如下：

```
type CopyObjectArgs struct {
    ObjectMeta
    MetadataDirective string
    IfMatch           string
    IfNoneMatch       string
    IfModifiedSince   string
    IfUnmodifiedSince string
}

type ObjectMeta struct {
	CacheControl       string
	ContentDisposition string
	ContentEncoding    string
	ContentLength      int64
	ContentRange       string
	ContentType        string
	ContentMD5         string
	ContentSha256      string
	Expires            string
	LastModified       string
	ETag               string
	UserMeta           map[string]string
	StorageClass       string
	NextAppendOffset   string
	ObjectType         string
}

type CopyObjectResult struct {
    LastModified string
    ETag         string
}
```

用户调用相关接口之后可以直接使用返回值对象的字段名访问相应的值。

BOS Client将上述原始API进行了封装，最大程度的方便用户使用各个API，全部
定义于`services/bos/client.go`文件中。针对同一个API一般提供多
种形式调用，以`GetObject` API举例来说明，在BOS Client对象上

  1. 首先提供了`GetObject`方法，直接调用原始API
  2. 封装`BasicGetObject`方法，仅使用必需参数调用
  3. 封装`BasicGetObjectToFile`方法，仅使用必须参数调用，将下载对象存入文件

用户使用BOS Client对象可以直接使用原始API，也可以使用封装的易用接口，所
有其他API的封装请使用`go doc`工具生成之后自行阅览，各接口的命名都具有自
说明语义。

### 配置BOS Client

用户创建BOS服务的Client对象之后，可以直接使用该对象的导出字段`Config`进
行个性化配置，该字段包含了如下配置项：

配置项名称 |  类型   | 含义
-----------|---------|--------
Endpoint   |  string | 请求服务的域名
ProxyUrl   |  string | 客户端请求的代理地址
Region     |  string | 请求资源的区域
UserAgent  |  string | 用户名称，HTTP请求的User-Agent头
Credentials| \*auth.BceCredentials | 请求的鉴权对象，分为普通AK/SK与STS两种
SignOption | \*auth.SignOptions    | 认证字符串签名选项
Retry      | RetryPolicy | 连接重试策略
ConnectionTimeoutInMillis| int     | 连接超时时间，单位毫秒，默认50秒

说明：

  1. `Credentials`字段使用`auth.NewBceCredentials`与`auth.NewSessionBceCredentials`函数
     创建，默认使用前者，后者为使用STS鉴权时使用，详见上一小节。
  2. `SignOption`字段为生成签名字符串时的选项，详见下表说明：

名称          | 类型  | 含义
--------------|-------|-----------
HeadersToSign |map[string]struct{} | 生成签名字符串时使用的HTTP头
Timestamp     | int64 | 生成的签名字符串中使用的时间戳，默认使用请求发送时的值
ExpireSeconds | int   | 签名字符串的有效期

     其中，HeadersToSign默认为`Host`，`Content-Type`，`Content-Length`，`Content-MD5`；
     TimeStamp一般为零值，表示使用调用生成认证字符串时的时间戳，用户一般不应该明确指定
     该字段的值；ExpireSeconds默认为1800秒即30分钟。
  3. `Retry`字段指定重试策略，目前支持两种：不重试；`BackOff`重试。默认使用后者。
     `BackOff`重试策略是指定最大重试次数、最长重试时间和重试基数，按照重试基数
     乘以2的指数级增长的方式进行重试，直到达到最大重试测试或者最大重试时间为止。

下面分别从各个配置项使用代码举例说明。

#### 配置HTTPS

```
AK, SK := <your-access-key-id>, <your-secret-access-key>

// 默认使用http协议
ENDPOINT := "gz.bcebos.com" // "http://gz.bcebos.com"也可
clientUseHttp, _ := bos.NewClient(AK, SK, ENDPOINT)

// 使用https协议
ENDPOINT = "https://gz.bcebos.com"
clientUseHttps, _ := bos.NewClient(AK, SK, ENDPOINT)
```

#### 配置用户代理

```
AK, SK := <your-access-key-id>, <your-secret-access-key>
ENDPOINT := "bj.bcebos.com"
client, _ := bos.NewClient(AK, SK, ENDPOINT)
client.Config.ProxyUrl = "127.0.0.1:8888"
```

#### 配置网络请求参数

```
AK, SK := <your-access-key-id>, <your-secret-access-key>
ENDPOINT := "bj.bcebos.com"
client, _ := bos.NewClient(AK, SK, ENDPOINT)

// 配置不进行重试，默认为Back Off重试
client.Config.Retry = bce.NewNoRetryPolicy()

// 配置连接超时时间
client.Config.ConnectionTimeoutInMillis = 30 * 1000 // 30秒
```

#### 配置认证机制

支持AK/SK和STS两种，默认为AK/SK认证；使用STS鉴权详见前文`通过STS方式访问BOS`一节。

#### 配置生成签名字符串选项

```
AK, SK := <your-access-key-id>, <your-secret-access-key>
ENDPOINT := "bj.bcebos.com"
client, _ := bos.NewClient(AK, SK, ENDPOINT)

// 配置签名使用的HTTP请求头为`Host`
headersToSign := map[string]struct{}{"Host": struct{}{}}
client.Config.SignOption.HeadersToSign = HeadersToSign

// 配置签名的有效期为30秒
client.Config.SignOption.ExpireSeconds = 30
```

## Bucket

Bucket既是BOS上的命名空间，也是计费、权限控制、日志记录等高级功能的管理实体。

- Bucket名称在所有区域中具有全局唯一性，且不能修改。

> 说明：
> 百度云目前开放了多区域支持，请参考区域选择说明。
> 目前支持“华北-北京”、“华南-广州”和“华东-苏州”三个区域。北京区域：
> http://bj.bcebos.com，广州区域：http://gz.bcebos.com，苏州区域：http://su.bcebos.com。

- 存储在BOS上的每个Object都必须包含在一个Bucket中。
- 一个用户最多可创建100个Bucket，但每个Bucket中存放的Object的数量和大小总
  和没有限制，用户不需要考虑数据的可扩展性。

### Bucket命名规范

Bucket的命名有以下规范：
- 只能包括小写字母，数字，短横线（-）。
- 必须以小写字母或者数字开头。
- 长度必须在3-63字节之间。

### 新建Bucket

新建Bucket的接口为`PutBucket`：
```
if loc, err := bosClient.PutBucket(<your-bucket-name>); err != nil {
    fmt.Println("create bucket failed:", err)
} else {
    fmt.Println("create bucket success at location:", loc)
}
```

> 注意：由于Bucket的名称在所有区域中是唯一的，所以需要保证bucketName不与其他所有区域上的Bucket名称相同。

### 查看Bucket列表

如下代码可以列出用户的所有Bucket：

```
if res, err := bosClient.ListBuckets(); err != nil {
    fmt.Println("list buckets failed:", err)
} else {
    fmt.Println("owner:", res.Owner)
    for i, b := range res.Buckets {
        fmt.Println("bucket", i)
        fmt.Println("    Name:", b.Name)
        fmt.Println("    Location:", b.Location)
        fmt.Println("    CreationDate:", b.CreationDate)
    }
}
```

### 删除Bucket

如下代码可以删除一个Bucket：

```
err := bosClient.DeleteBucket(bucketName)
```

> 注意：如果Bucket不为空（即Bucket中有Object存在），则无法删除，必须先删除所有Object后才能删除Bucket。

### 获取Bucket的位置

如下代码可以获取一个Bucket位置：

```
location, err := bosClient.GetBucketLocation(bucketName)
```

### 判断Bucket是否存在

如下代码能够判断一个Bucket是否存在：

```
exists, err := bosClient.DoesBucketExist(bucketName)
```

### Bucket权限控制

#### 设置Bucket的Canned ACL
如下代码设置Bucket的Canned ACL，支持`private`、`public-read`、`public-read-write`三种。

```
err := bosClient.PutBucketAclFromCanned(bucketName, "public-read")
```

#### 设置用户对Bucket的访问权限

BOS除了支持设置整个Bucket的Canned ACL之外，还可以设置更复杂的针对单个用
户的ACL权限设置，参考如下四种方式设置：

```
// 1. 直接上传ACL流
aclBodyStream := bce.NewBodyFromFile("<path-to-acl-file>")
err := bosClient.PutBucketAcl(bucket, aclBodyStream)

// 2. 直接使用ACL json字符串
aclString := `{
    "accessControlList":[
        {
            "grantee":[{
                "id":"e13b12d0131b4c8bae959df4969387b8"
            }],
            "permission":["FULL_CONTROL"]
        }
    ]
}`
err := bosClient.PutBucketAclFromString(bucket, aclString)

// 3. 使用ACL文件
err := bosClient.PutBucketAclFromFile(bucket, "<acl-file-name>")

// 4. 使用ACL struct对象设置
grantUser1 := api.GranteeType{"<user-id-1>"}
grantUser2 := api.GranteeType{"<user-id-2>"}
grant1 := api.GrantType{
            Grantee: []api.GranteeType{grantUser1},
            Permission: []string{"FULL_CONTROL"}
        }
grant2 := api.GrantType{
            Grantee: []api.GranteeType{granteUser2},
            Permission: []string{"READ"}
        }
grantArr := make([]api.GranteType)
grantArr = append(grantArr, grant1)
grantArr = append(grantArr, grant2)
args := &api.PutBucketAclArgs{grantArr}
err := bosClient.PutBucketAclFromStruct(bucketName, args)
```

> 注意：Permission中的权限设置包含三个值：`READ`、`WRITE`、`FULL_CONTROL`，它们分
> 别对应相关权限。具体内容可以参考BOS API文档。ACL规则比较复杂，直接编辑ACL的文
> 件或JSON字符串比较困难，因此上述第四种方式方便使用代码创建ACL规则，易于理解。

#### 设置访问的Refer

跨域资源共享(CORS)允许WEB端的应用程序访问不属于本域的资源。BOS提供的Bucket
权限控制可以让开发者设置跨域访问的各种权限。示例代码如下：

```
aclString := `{
    "accessControlList":[
        {
            "grantee":[{"id":"*"}],
            "permission":["FULL_CONTROL"]
            "condition" {
                "referer": {"stirngEquals": ["http://<your-domain>", "https://<your-domain>"]}
            }
        }
    ]
}`
err := bosClient.PutBucketAclFromString(bucket, aclString)
```

#### 获取权限信息

如下代码获取Bucket的权限信息：

```
res, err := bosClient.GetBucketAcl(bucketName)
```

### Bucket日志管理

用户可以指定访问的Bucket的日志存放的位置。

#### 开启并设置放置Bucket和日志文件前缀

访问日志的命名规则和日志格式详见Bucket相关API文档，下面的示例代码可以
开启和设置访问日志的位置和前缀：

```
// 1. 原始接口
loggingStrem := bce.NewBodyFromFile("<path-to-logging-setting-file>")
err := bosClient.PutBucketLogging(bucketName, loggingStream)

// 2. 从JSON字符串设置
loggingStr := `{"targetBucket": "logging-bucket", "targetPrefix": "my-log/"}`
err := bosClient.PutBucketLoggingFromString(bucketName, loggingStr)

// 3. 从参数对象设置
args := new(api.PutBucketLoggingArgs)
args.TargetBucket = "logging-bucket"
args.TargetPrefix = "my-log/"
err := bosClient.PutBucketLoggingFromStruct(bucketName, args)
```

#### 获取和删除日志配置信息

下面的代码分别给出了如何获取和删除日志配置信息：

```
// 获取日志配置信息
res, err := bosClient.GetBucketLogging(bucketName)
fmt.Println(res.Status)
fmt.Println(res.TargetBucket)
fmt.Println(res.TargetPrefix)

// 删除日志配置信息
err := bosClient.DeleteBucketLogging(bucketName)
```

### Bucket生命周期管理

一个数据是有其生命周期的，从创建到归档到删除可以认为是一个完整的循
环。创建之初的数据往往需要频繁访问读取，之后迅速冷却归档，最终被删
除。生命周期管理就是对象存储服务帮助用户自动化管理数据的生命周期。

通常可以服务于以下场景：

  - 数据达到一定寿命后自动归档或删除。
  - 指定时间执行操作。

#### 设置生命周期规则

生命周期的规则有一套完整的语法，详细参考相关API文档。如下代码设置了
一个在指定日期之后进行删除操作的生命周期规则：

```
ruleStr := `{
    "rule": [
        {
            "id": "delete-rule-1",
            "status": "enabled",
            "resource": ["my-bucket/abc*"],
            "condition": {
                "time": {
                    "dateGreaterThan": "2018-01-01T00:00:00Z"
                }
            },
            "action": {
                "name": "DeleteObject"
            }
        }
    ]
}`

// 1. 原始接口，通过stream设置
body, _ := bce.NewBodyFromString(ruleStr)
err := bosClient.PutBucketLifecycle(bucketName, body)

// 2. 直接传入字符串
err := bosClient.PutBucketLifecycleFromString(bucketName, ruleStr)
```

#### 获取和删除生命周期配置

下面代码分别给出了获取和删除生命周期配置的示例：

```
// 获取
res, err := bosClient.GetBucketLifecycle(bucketName)

// 删除
err := bosClient.DeleteBucketLifecycle(bucketName)
```

### Bucket存储类型管理

每个Bucket会有自身的存储类型，如果该Bucket下的Object上传时未指定存储类型
则会默认继承该Bucket的存储类型。

#### 设置Bucket存储类型

```
storageClass := "STANDARD_IA"
err := bosClient.PutBucketStorageclass(bucketName, storageClass)
```

#### 获取Bucket存储类型

```
storageClass, err := bosClient.GetBucketStorageclass(bucketName)
```

## Object

在BOS中，用户操作的基本数据单元是Object。Object包含Key、Meta和Data。
其中，Key是Object的名字；Meta是用户对该Object的描述，由一系列Name-Value
对组成；Data是Object的数据。

### Object命名规范

Object的命名规范如下：

- 使用UTF-8编码。
- 长度必须在1-1023字节之间。
- 首字母不能为'/'。

### 上传Object

#### 基本上传

GO SDK支持如下形式的上传，参考如下代码：

```
// 1. 从本地文件上传
etag, err := bosClient.PutObjectFromFile(bucketName, objectName, fileName, nil)

// 2. 从字符串上传
str := "test put object"
etag, err := bosClient.PutObjectFromString(bucketName, objectName, str, nil)

// 3. 从字节数组上传
byteArr := []byte("test put object")
etag, err := bosClient.PutObjectFromBytes(bucketName, objectName, byteArr, nil)

// 4. 从数据流上传
bodyStream, err := bce.NewBodyFromFile(fileName)
etag, err := bosClient.PutObject(bucketName, objectName, bodyStream, nil)

// 5. 使用基本接口，提供必需参数从数据流上传
bodyStream, err := bce.NewBodyFromFile(fileName)
etag, err := bosClient.BasicPutObject(bucketName, objectName, bodyStream)
```

上述前四个接口的最后一个参数为可选参数，用户可进行自定义设置，为空
表示使用默认值。上述上传Object接口只支持不超过5GB的Object上传。在
请求处理成功后，BOS会在Header中返回Object的ETag作为文件标识。

#### 设置上传Object的参数

用户可以使用参数对象`PutObjectArgs`设置上传Object的参数。目前支持的参数
包括："CacheControl"、"ContentDisposition"、"Expires"、"ContentMD5"、"UserMeta"、
"ContentType"、"ContentLength"、"ContentSha256"和"StorageClass"。

```
// 设置存储类型为低频存储，标准存储和冷存储类似
args := new(api.PutObjectArgs)
args.StorageClass = api.STORAGE_CLASS_STANDARD_IA
etag, err := bosClient.PutObject(bucketName, objectName, bodyStream, args)

// 设置用户自定义元数据
args := new(api.PutObjectArgs)
args.UserMeta = map[string]string{"name": "my-custom-metadata"}
etag, err := bosClient.PutObject(bucketName, objectName, bodyStream, args)
```

> 注意：用户上传对象时会自动SDK会自动设置ContentLength和ContentMD5，用来保证
> 数据的正确性。如果用户自行设定ContentLength，必须为大于等于0且小于等于实际
> 对象大小的数值，从而上传截断部分的内容，为负数或大于实际大小均报错。

#### 修改Object的Metadata

BOS修改Object的Metadata通过拷贝Object实现。即拷贝Object的时候，把目的Bucket
设置为源Bucket，目的Object设置为源Object，并设置新的Metadata，通过拷贝自身
实现修改Metadata的目的。如果不设置新的Metadata，则报错。

```
// 创建CopyObjectArgs对象用于设置可选参数
args := new(api.CopyObjectArgs)

// 设置Metadata参数值，具体字段请参考官网说明
args.LastModified = "Wed, 29 Nov 2017 13:18:08 GMT"
args.ContentType = "text/json"

// 使用CopyObject接口修改Metadata，源对象和目的对象相同
res, err := bosClient.CopyObject(bucket, object, bucket, object, args)
```

#### 使用Append方式上传

BOS支持AppendObject，即以追加写的方式上传文件，适用场景如日志追加及直播
等实时视频文件上传。通过AppendObject操作创建的Object类型为Appendable
Object，可以对该Object追加数据；而通过PutObject上传的Object是Normal Object，
不可进行数据追加写。如果不设置offset，默认为0，也就是会覆盖原来的对象。
AppendObject大小限制为0~5G，下面为示例：

```
// 1. 原始接口上传，设置为低频存储和追加的偏移
args := new(api.AppendObjectArgs)
args.StorageClass = api.STORAGE_CLASS_STANDARD_IA
args.Offset = 1024
res, err := bosClient.AppendObject(bucketName, objectName, bodyStream, args)

// 2. 封装的简单接口，仅支持设置offset
res, err := bosClient.SimpleAppendObject(bucketName, objectName, bodyStream, offset)

// 3. 封装的从字符串上传接口，仅支持设置offset
res, err := bosClient.SimpleAppendObjectFromString(bucketName, objectName, "abc", offset)

// 4. 封装的从给出的文件名上传文件的接口，仅支持设置offset
res, err := bosClient.SimpleAppendObjectFromFile(bucketName, objectName, "<path-to-local-file>", offset)
```

### 抓取Object

BOS支持用户提供的url自动抓取相关内容并保存为指定Bucket的指定名称的Object。

```
// 1. 原始接口抓取，提供可选参数
args := new(api.FetchObjectArgs)
args.FetchMode = api.FETCH_MODE_ASYNC
res, err := bosClient.FetchObject(bucket, object, url, args)

// 2. 基本抓取接口，默认为同步抓取
res, err := bosClient.BasicFetchObject(bucket, object, url)

// 3. 易用接口，直接指定可选参数
res, err := bosClient.SimpleFetchObject(bucket, object, url,
        api.FETCH_MODE_ASYNC, api.STORAGE_CLASS_STANDARD_IA)

```

相关详细说明见官网`FetchObject`API的文档。

### 查看Bucket中的Object列表

当用户完成一系列上传后，可能会需要查看在指定Bucket中的全部Object，可以
通过如下代码实现：

```
// 1. 原始接口，参数对象设置返回最多50个
args := new(api.ListObjectsArgs)
args.MaxKeys = 50
res, err := bosClient.ListObjects(bucket, args)
fmt.Println(res.Prefix)
fmt.Println(res.Delimiter)
fmt.Println(res.Marker)
fmt.Println(res.NextMarker)
fmt.Println(res.MaxKeys)
fmt.Println(res.IsTruncated)
for _, obj := range res.Contents {
    fmt.Println(obj.Key)
    fmt.Println(obj.LastModified)
    fmt.Println(obj.ETag)
    fmt.Println(obj.Size)
    fmt.Println(obj.StorageClass)
    fmt.Println(obj.Owner)
}

// 2. 易用接口，不需创建参数对象
res, err := bosClient.SimpleListObjects(bucket, prefix, maxKeys, marker, delimiter)
```

### 获取Object

#### 获取对象内容

BOS Client提供了多种接口用来获取一个对象的内容，示例代码如下：

```
// 1. 原始接口，可提供可选参数
responseHeaders := map[string]string{"ContentType": "image/gif"}
rangeStart = 1024
rangeEnd = 2048
res, err := bosClient.GetObject(bucketName, objectName, responseHeaders, rangeStart, rangeEnd)
buf := new(bytes.Buffer)
io.Copy(buf, res.Body)
// 只指定start
res, err := bosClient.GetObject(bucketName, objectName, responseHeaders, rangeStart)
// 不指定range
res, err := bosClient.GetObject(bucketName, objectName, responseHeaders)
// 不指定返回可选头部
res, err := bosClient.GetObject(bucketName, objectName, nil)

// 2. 基本接口，获取一个对象
res, err := bosClient.BasicGetObject(bucketName, objectName)

// 3. 基本接口，下载一个对象到本地文件
res, err := bosClient.BasicGetObjectToFile(bucketName, objectName, "path-to-local-file")
```

上述获取对象返回的结果包含了该对象的metadata信息和流，用户可以直接
访问相关字段，如获取对象的ETag可以直接使用`res.ETag`即可。用户可以对返
回的流进行相关操作，示例中直接拷贝到了一个缓冲区`io.Copy(buf, res.Body)`。
用户可以通过设置可选参数中的`RangeStart`、`RangeEnd`参数实现分段下载和断点
续传，详见`DownloadSuperFile`接口实现的并发下载。

#### 获取对象的Metadata

如果用户只需要获取某个对象的metadata信息，不需要下载整个对象的内容时，
可以使用`GetObjectMeta`接口来实现：

```
res, err := bosClient.GetObjectMeta(bucketName, objectName)
fmt.Println("%+v", res)
```

相关metadata字段可以直接通过`res.xxx`获取，如获取`LastModified`可以通过
`res.LastModified`。具体支持的metadata字段见官网API介绍。

#### 获取对象的签名URL

用户可以为一个对象生成一个签名后的URL，可以指定请求方法，签名选项和
过期时间。示例代码如下：

```
// 1. 原始接口，可设置所有参数
url := bosClient.GeneratePresignedUrl(bucketName, objectName, expire, method, headers, params)

// 2. 基本接口，默认为`GET`方法，headers和params为空
url := bosClient.BasicGeneratePresignedUrl(bucketName, objectName, expire)
```

`expire`为指定的URL有效时长，单位为秒，时间从当前时间算起。如果要设置为
永久不失效的时间，可以将其设置为-1，不可设置为其他负数。

### 拷贝Object

在`修改Object的Metadata`小节已经给出了利用`CopyObject`接口修改meta信息，如果
源对象和目的对象不相同，就会进行实际的拷贝。

当前BOS的CopyObject接口是通过同步方式实现的。同步方式下，BOS端会等待Copy实
际完成才返回成功。同步Copy能帮助用户更准确的判断Copy状态，但用户感知的复制
时间会变长，且复制时间和文件大小成正比。同步Copy方式更符合业界常规，提升
了与其它平台的兼容性。同步Copy方式还简化了BOS服务端的业务逻辑，提高了服务效率。

```
// 1. 原始接口，可设置拷贝参数
res, err := bosClient.CopyObject(bucketName, objectName, srcBucket, srcObject, nil)

// 2. 忽略拷贝参数，使用默认
res, err := bosClient.BasicCopyObject(bucketName, objectName, srcBucket, srcObject)
```

支持的拷贝参数详见官网API文档，定义如下：

```
type CopyObjectArgs struct {
	ObjectMeta
	MetadataDirective string
	IfMatch           string
	IfNoneMatch       string
	IfModifiedSince   string
	IfUnmodifiedSince string
}
```

### 删除Object

删除Object支持一次删除一个，或者一次请求批量删除多个。

如下示例代码删除一个Object：

```
err := bosClient.DeleteObject(bucketName, objectName)
```

如下示例代码给出了四种不同的方式，来一次删除多个Object：

```
// 1. 原始接口，提供多个Object的List Stream
res, err := bosClient.DeleteMultipleObjects(bucket, object, objectListStream)

// 2. 提供json字符串删除
objectList := `{
    "objects":[
        {"key": "aaa"},
        {"key": "bbb"}
    ]
}`
res, err := bosClient.DeleteMultipleObjectsFromString(bucket, object, objectList)

// 3. 提供删除Object的List对象
deleteObjectList := make([]api.DeleteObjectArgs, 0)
deleteObjectList = append(deleteObjectList, api.DeleteObjectArgs{"aaa"})
deleteObjectList = append(deleteObjectList, api.DeleteObjectArgs{"bbb"})
multiDeleteObj := &api.DeleteMultipleObjectsArgs{deleteObjectList}
res, err := bosClient.DeleteMultipleObjectsFromStruct(bucket, object, multiDeleteObj)

// 4. 直接提供待删除Object的名称列表
deleteObjects := []string{"aaa", "bbb"}
res, err := bosClient.DeleteMultipleObjectsFromKeyList(bucket, object, deleteObjects)

```

说明：

> 一次删除多个Object的时候，返回的结果里包含了未删除成功的Object名称列
> 表。删除部分对象成功时`err`是`nil`，`res`里包含了为删除的名称列表，如
> 果`err`与`res`均为`nil`则表明删除了全部Object成功。

## Object的分块操作

对于容量过大的Object，提供了分块操作的API将大对象进行分块，然后按各个
分块为单位执行相关操作，目前BOS服务提供了分块上传分块拷贝。

所有的分块操作都分为三个步骤：
  1. 初始化一个分块操作的ID：InitiateMultipartUpload
  2. 执行分块操作：UploadPart/UploadPartCopy
  3. 确认完成整个分块操作：CompleteMultipartUpload
如果在执行分块操作的过程中出现了错误需要临时取消这个分块操作，或者
需要在上传过程中查看上传进度，还提供了AbortMultipartUpload、ListParts
和ListMultipartUploads接口。

### 分块操作参数控制

BOS Client提供了对所有分块操作进行控制的参数：

- MultipartSize：每个分块的大小，默认为10MB，最小不得低于5MB
- MaxParallel：分块操作的并发数，默认为10

下面的示例代码设置了分块的大小为20MB，并发数为100：

```
client := bos.NewClient(<your-ak>, <your-sk>, <endpoint>)
client.MultipartSize = 20 * (1 << 10)
client.MaxParallel = 100
```

除了上述参数外，还会对设置的每个分块数进行1MB对齐，同时限制是最大分
块数目不得超过10000，如果分块较小导致分块数超过这个上限会自动调整分
块大小。这些参数的设置会对所有分块操作生效。

### Object的分块上传

除了通过putObject接口上传文件到BOS以外，BOS还提供了另外一种上传模
式 —— Multipart Upload。用户可以在如下的应用场景内（但不仅限于此），
使用Multipart Upload上传模式，如：

- 需要支持断点上传。
- 上传超过5GB大小的文件。
- 网络条件较差，和BOS的服务器之间的连接经常断开。
- 需要流式地上传文件。
- 上传文件之前，无法确定上传文件的大小。

下面将一步步介绍Multipart Upload的实现。假设有一个文件，本地路径为
`/path/to/file.zip`，由于文件比较大，将其分块传输到BOS中。

#### 初始化

使用`BasicInitiateMultipartUpload`方法来初始化一个基本的分块上传事件，获取
分块上传的`UploadId`：

```
res, err := bosClient.BasicInitiateMultipartUpload(bucketName, objectKey)
fmt.Println(res.UploadId)
```

也可使用`InitiateMultipartUpload`接口设置其他参数：`Content-Type`、`Cache-Control`、
`Content-Disposition`、`Expire`、`StorageClass`。下面分别初始化一个低频存储的分
块事件和冷存储的分块事件：

```
// 低频存储
res, err := bosClient.InitiateMultipartUpload(bucketName, objectKey, contentType,
        &api.InitiateMultipartUploadArgs{StorageClass: api.STORAGE_CLASS_STANDARD_IA})

// 冷存储
res, err := bosClient.InitiateMultipartUpload(bucketName, objectKey, contentType,
        &api.InitiateMultipartUploadArgs{StorageClass: api.STORAGE_CLASS_COLD})
```

#### 执行分块上传
```
    file, _ := os.Open("/path/to/file.zip")
    result := make([]api.UploadInfoType)
    for i := 0; i < partNum; i++  {
        partBody, _ := bce.NewBodyFromSectionFile(file, offset[i], uploadSize)
        etag, err := bosClient.BasicUploadPart(bucketName, objectKey, uploadId, i+1, partBody)
        result = append(result, api.UploadInfoType{partNum, etag})
    }
```

这里使用了`BasicUploadPart`接口，只提供必需参数，也可以使用`UploadPart`接口
提供`api.UploadPartArgs`对象来设置`Content-MD5`、`x-bce-content-sha256`等参数。

> 注意：这两个接口中的`PartNumber`参数是从`1`开始计算的。

#### 结果控制

完成阶段需要使用第二步获取的数据作为参数进行最终的确认。

```
completeArgs := &api.CompleteMultipartUploadArgs{result}
res, _ := bosClient.CompleteMultipartUploadFromStruct(
        bucketName, objectKey, uploadId, completeArgs, nil)
fmt.Println(res.Location)
fmt.Println(res.Bucket)
fmt.Println(res.Key)
fmt.Println(res.ETag)
```

如果执行分块操作过程中出现错误，可以直接终止当前操作：

```
bosClient.AbortMultipartUpload(bucketName, objectKey, uploadId)
```

另外，可以在上传过程中查询当前成功上传的分块和未完成的分块：

```
// 使用基本接口列出当前上传成功的分块
bosClient.BasicListParts(bucketName, objectKey, uploadId)

// 使用原始接口提供list参数，列出当前上传成功的最多100个分块
bosClient.ListParts(bucketName, objectKey, uploadId, &api.ListPartsArgs{MaxParts: 100})

// 列出给定bucket下所有未完成的分块信息
res, err := BasicListMultipartUploads(bucketName)
```

上述示例是使用API依次实现，并且没有并发执行，如果需要加快速度需要用
户实现并发上传的部分。为了方便用户使用，BOS Client特封装了分块上传的
并发接口`UploadSuperFile`：

- 接口：`UploadSuperFile(bucket, object, fileName, storageClass string) error`
- 参数:
    - bucket: 上传对象的bucket的名称
    - object: 上传对象的名称
    - fileName: 本地文件名称
    - storageClass: 上传对象的存储类型，默认标准存储
- 返回值:
    - error: 上传过程中的错误，成功则为空

用户只需给出`bucket`、`object`、`filename`即可进行并发上传，同时也可指定上
传对象的`storageClass`。

### Object的分块拷贝

分块拷贝操作的流程与除了执行的操作是拷贝之外，其余与分块上传完全相
同，提供了原始API`UploadPartCopy`能够设置可选参数，也提供了基本API`BasicUploadPart`
只需提供必需参数。分块拷贝示例如下：

```
optArgs := &api.UploadPartCopyArgs{}
for i := 0; i < partNum; i++  {
    res, _ := bosClient.UploadPartCopy(bucket, object, srcBucket, srcObject,
            uploadId, i+1, optArgs)
    result = append(result, api.UploadInfoType{i+1, res.Etag})
}
```

### Object的并发下载

BOS没有提供大对象的分块下载操作，但是对于下载接口`GetObject`提供了
Range支持，因此可以利用这个特性实现对大文件的并发下载。BOS Client据此
封装了一个并发下载的接口`DownloadSuperFile`：

- 接口：`DownloadSuperFile(bucket, object, fileName string) error`
- 参数:
    - bucket: 下载对象所在bucket的名称
    - object: 下载对象的名称
    - fileName: 该对象保存到本地的文件名称
- 返回值:
    - error: 下载过程中的错误，成功则为空

## 日志

GO SDK自行实现了支持六个级别、三种输出（标准输出、标准错误、文件）、
基本格式设置的日志模块，代码见`baidubce/util/log`。输出为文件时支持
设置五种日志滚动方式（不滚动、按天、按小时、按分钟、按大小），此时
还需设置输出日志文件的目录。详见下文。

### SDK日志
SDK使用`baidubce/util/log`里包级别的全局日志对象，该对象默认情况下不
记录日志，如果需要输出SDK相关日志需要用户自定指定输出方式和级别，详
见如下示例：
```
// 指定输出到标准错误，输出INFO及以上级别
log.SetLogHandler(log.STDERR)
log.SetLogLevel(log.INFO)

// 指定输出到标准错误和文件，DEBUG及以上级别，以1GB文件大小进行滚动
log.SetLogHandler(log.STDERR | log.FILE)
log.SetLogDir("/tmp/gosdk-log")
log.SetRotateType(log.ROTATE_SIZE)
log.SetRotateSize(1 << 30)

// 输出到标准输出，仅输出级别和日志消息
log.SetLogHandler(log.STDOUT)
log.SetLogFormat([]string{log.FMT_LEVEL, log.FMT_MSG})
```

说明：
  1. 日志默认输出级别为`DEBUG`
  2. 如果设置为输出到文件，默认日志输出目录为`/tmp`，默认按小时滚动
  3. 如果设置为输出到文件且按大小滚动，默认滚动大小为1GB
  4. 默认的日志输出格式为：`FMT_LEVEL, FMT_LTIME, FMT_LOCATION, FMT_MSG`

### 项目使用

该日志模块无任何外部依赖，用户使用GO SDK开发项目，可以直接引用该日志
模块自行在项目中使用，用户可以继续使用GO SDK使用的包级别的日志对象，
也可创建新的日志对象，详见如下示例：

```
// 直接使用包级别全局日志对象（会和GO SDK日志输出关联起来）
log.SetLogHandler(log.STDERR)
log.Debugf("%s", "logging message using the log package in the BOS go sdk")

// 创建新的日志对象（与GO SDK日志输出分离）
myLogger := log.NewLogger()
myLogger.SetLogHandler(log.FILE)
myLogger.SetLogDir("/home/log")
myLogger.SetRotateType(log.ROTATE_SIZE)
myLogger.Info("this is my own logger from the BOS go sdk")
```

## 错误处理

GO语言以error类型标识错误，BOS支持两种错误见下表，定义于`baidubce/bce`
目录下。（目录结构见"安装SDK工具包"节）

错误类型        |  说明
----------------|-------------------
BceClientError  | 用户操作产生的错误
BceServiceError | BOS服务返回的错误

用户使用SDK调用BOS相关接口，除了返回所需的结果之外还会返回错
误，用户可以获取相关错误进行处理。实例如下：
```
// bosClient 为已创建的BOS Client对象
bucketLocation, err := bosClient.PutBucket("test-bucket")
if err != nil {
    switch realErr := err.(type) {
    case *bce.BceClientError:
        fmt.Println("client occurs error:", realErr.Error())
    case *bce.BceServiceError:
        fmt.Println("service occurs error:", realErr.Error())
    default:
        fmt.Println("unknown error:", err)
    }
} else {
    fmt.Println("create bucket success, bucket location:", bucketLocation)
}
```

## 版本变更记录

### v0.9.1 [2018-1-4]

首次发布：

 - 创建、查看、罗列、删除Bucket，获取位置和判断是否存在
 - 支持管理Bucket的生命周期、日志、ACL、存储类型
 - 上传、下载、删除、罗列Object，支持分块上传、分块拷贝
 - 提供AppendObject功能和FetchObject功能
 - 封装并发的下载和分块上传接口
