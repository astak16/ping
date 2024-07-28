## 介绍

此代码仅供学习使用

下载代码

```bash
git clone https://github.com/astak16/ping.git
```

打包

```bash
go build .
```

运行

```
./ping www.baidu.com

./ping -i 2000 www.baidu.com

./ping -w 2000 www.baidu.com
```

## 解析域名

我们在使用 `ping www.baidu.com` 命令时，会看到下面的输出

```bash
PING www.a.shifen.com (153.3.238.102) 56(84) bytes of data.
```

`www.a.shifen.com` 和 `153.3.238.102` 一个是 `cname` 记录，一个是 `ipv4` 地址

什么是 `cname` 记录？

`cname` 全称 `Canonical Name Record`，即真是名称记录，用于将域名映射到另一个域名

`cname` 的作用是：

1. 多个域名都指向同一个别名，当 `IP` 变化时，只需要更新该别名的 `IP` 地址(`A` 记录)，其他域名不需要改变
2. 有的域名不属于自己，例如 `CDN` 服务，服务商提供的就是一个 `CNAME`，将自己的 `CDN` 域名绑定到 `CNAME` 上，`CDN` 服务提供商就可以根据地区、负载均衡、错误转移等情况，动态改别名的 `A` 记录，不影响自己 `CDN` 到 `CNAME` 的映射。

在 `go` 中，可以通过 `net` 包解析域名

- 查看 `cname` 通过 `net.LookupCNAME(domain)`
  ```go
  cname, err := net.LookupCNAME("www.baidu.com")
  // cname: www.a.shifen.com.
  ```
- 查看 `ip` 通过 `net.LookupIP(domain)`
  ```go
  records, err := net.LookupIP("www.baidu.com")
  // records: [153.3.238.102 153.3.238.110 2408:873d:22:18ac:0:ff:b021:1393 2408:873d:22:1a01:0:ff:b087:eecc]
  ```

这里要注意一点，域名解析出来的 `cname` 可能背后还有 `cname`，比如 `www.zhihu.com`

```
www.zhihu.com
www.zhihu.com.ipv6.dsa.dnsv1.com.
1595096.sched.d0-dk.tdnsdp1.cn.
```

我们通过 `ping www.zhihu.com` 命令，看到的 `cname` 是 `1595096.sched.d0-dk.tdnsdp1.cn`

```bash
ping www.zhihu.com
PING 1595096.sched.d0-dk.tdnsdp1.cn (61.241.148.88): 56 data bytes
```

所以我们需要递归解析 `cname`，直到没有 `cname` 为止

解析域名的代码如下：

```go
type Info struct {
  Cname string
  Ip    []net.IP
}

func resolveDomain(domain string) (*Info, error) {
  records, err := net.LookupIP(domain)
  if err != nil {
    return nil, fmt.Errorf("查找IP时出错: %v", err)
  }

  cname, err := net.LookupCNAME(domain)
  if err != nil {
    // 如果找不到CNAME，就使用原始域名
    cname = domain + "."
  }

  return &Info{Cname: cname, Ip: records}, nil
}
func ResolveDomainWithTimeout(domain string, timeout time.Duration) (*Info, error) {
  startTime := time.Now()
  resultChan := make(chan *Info, 1)
  errorChan := make(chan error, 1)

  for {
    go func() {
      info, err := resolveDomain(domain)
      if err != nil {
        errorChan <- err
      } else {
        resultChan <- info
      }
    }()
    select {
    case result := <-resultChan:
      fmt.Println(result, "ss")
      if result.Cname == domain+"." {
        return result, nil
      }
      // CNAME 不匹配，继续解析新的域名
      domain = strings.TrimSuffix(result.Cname, ".")
    case err := <-errorChan:
      return nil, err
    case <-time.After(timeout - time.Since(startTime)):
      return nil, fmt.Errorf("域名解析超时")
    }

    // 检查是否超时
    if time.Since(startTime) >= timeout {
      return nil, fmt.Errorf("域名解析超时")
    }
  }
}

func ResolveDomain(initialDomain string) (string, []net.IP, error) {
  domain := initialDomain
  info, err := ResolveDomainWithTimeout(domain, 5*time.Second)
  if err != nil {
    return "", nil, err
  }
  domain = info.Cname[:len(info.Cname)-1] // 移除末尾的点
  return domain, info.Ip, nil
}
```

## 校验和

为什么要处理进位：

1. 校验和的目的：主要是检测数据传输过程中的错误，它需要能够捕捉到任何位的变化
2. `16` 位限制：在许多网络协议中，校验和字段被限制为 `16` 位，这是为了在保持一定错误检测能力的同时，减少额外的开销
3. 进位的处理：

- 当我们将多个 `16` 位数相加时，结果可能会超过 `16` 位
- 简单地截断到 `16` 位（忽略进位）会丢失信息，就会降低错误检测能力
- 因此，采用"回卷"（`wrap-around`）的方法：将高于 `16` 位的部分加回到低 `16` 位

4. 循环处理：

- 在某些情况下，加一次可能还会产生新的进位
- 所以我们需要重复这个过程，直到没有进位为止

最终结果：
这个过程确保了所有的位都被考虑在内，即使原始和超过了 `16` 位

例子：

1. 没有进位，假设数据：`0x1234`, `0x5678`, `0x9A`

```go
  0x1234
+ 0x5678
+ 0x009A  (填充一个字节)
-----------
  0x6946  (sum)

  0x6946
+    0x0  (没有进位)
-----------
  0x6946

  ~0x6946 = 0x96B9  (最终的校验和)
```

2. 有进位，假设数据：`0xA987`, `0x6543`, `0x21`

```go
  0xA987
+ 0x6543
+ 0x0021  (填充一个字节)
-----------
  0x10EEB  (sum，产生了进位)

  0x0EEB
+ 0x0001  (进位)
-----------
  0x0EEC

  ~0x0EEC = 0xF113  (最终的校验和)
```

算法：

1. 将数据按 `16` 位（`2` 字节）进行分组
2. 如果数据长度为奇数，最后一个字节单独处理
3. 将相邻的两个字节拼接成一个 `16` 位的数，然后相加
4. 对结果取反

用代码表示这个过程：

```go
func checkSum(data []byte) uint16 {
  // 计算数据的长度
  length := len(data)
  index := 0
  // 用 uint32 是为了避免数据出现溢出
  var sum uint32
  // 需要循环处理数据，如果数据长度为奇数，最后一个字节单独处理
  for length > 1 {
    // 求和的方式是将相邻两个字节拼接成一个 16 位的数，然后相加
    // 第一个数作为高 8 位，所以要左移 8 位
    // 第二个数作为低 8 位
    sum += uint32(data[index])<<8 + uint32(data[index+1])
    // 拿下一组数据
    length -= 2
    index += 2
  }

  // 如果最后 length 为 1，说明最后一个字节没有成对，直接作为低 8 位，加到 sum 上
  if length == 1 {
    sum += uint32(data[index])
  }

  // 因为 16 位的数最大是 65535，所以如果 sum 大于 65535，说明数据有溢出
  // 1. 对 sum 右移 16 位，取出溢出的值
  // 2. 将 sum 转为 16 位的数
  // 3. 将 1 中的值加到 2 中的值上
  // 4. 重复 1-3 步骤，直到 sum 不再溢出
  // 如何判断有没有溢出呢？
  // 		如果没有溢出，右移 16 位，的结果为 0
  // 		如果有溢出，右移 16 位，的结果不为 0
  hi := sum >> 16
  for hi != 0 {
    sum = hi + uint32(uint16(sum))
    hi = sum >> 16
  }
  // 对校验和取反
  // 校验和的反码，和校验和相加应该等于 0xffff，说明数据没有丢失(篡改)
  return uint16(^sum)
}
```

## 为什么取反能够检测错误

因为在数据传输过程中，一个比特(`bit`)发生了翻转(由 `0` 变成 `1`，或者由 `1` 变成了 `0`)

取反操作可以确保这种单比特错误在接收端能够被检测到

如果没有错误，那么校验和的反码和校验和相加应该等于 `0xffff`，说明数据没有丢失(篡改)

假设数据没有丢失

```go
data := []byte{0x01, 0x02, 0x03, 0x04}
```

根据算法得到校验和为 `0x0406`

```go
sum = uint32(0x01) << 8 + uint32(0x02) + uint32(0x03) << 8 + uint32(0x04)
    = 0x0102 + 0x0304
    = 0x0406
```

将校验和取反得到 `0xFBF9`

接收端收到数据计算校验和，与 `0xFBF9` 相加，如果结果为 `0xffff`

假设数据发生了变化，`0x01` 变成了 `0x11`

```go
data := []byte{0x11, 0x02, 0x03, 0x04}
```

根据算法得到校验和为 `0x1406`

```go
newSum = uint32(0x11) << 8 + uint32(0x02) + uint32(0x03) << 8 + uint32(0x04)
      = 0x1102 + 0x0304
      = 0x1406
```

将校验和 `0x1406` 和 `0xFBF9` 相加，得到结果 `0x0FFF`，说明数据发生了变化

## buf 含义

`conn` 是一个 `net.Conn` 接口，`Read` 方法从连接中读取数据

`buf` 是一个字节切片，用于存储读取到的数据

- `size` 是数据的大小，
- `20` 是 `IP` 头部的大小
- `8` 是 `ICMP` 头部的大小
- `20` 是 `padding` 的大小，冗余数据

```go
buf := make([]byte, size+20+8+20)
_, err = conn.Read(buf)

// buf 输出
[69 0 0 84 127 172 0 0 63 1 200 126 153 3 238 102 172 18 0 2 0 0 255 253 0 1 0 1 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0]
```

这个 `buf` 分为四部分：

- `ipv4` 头部(`20` 字节)
- `ICMP` 头部(`8` 字节)
- 数据(`size` 字节)
- `padding`(`20` 字节)

数据和 `padding` 部分可以忽略，重点看下 `ipv4` 和 `ICMP` 头部这两部分

`ipv4` 头部：

| 值              | 说明                                       | 含义                                                                        |
| :-------------- | :----------------------------------------- | :-------------------------------------------------------------------------- |
| `69`            | 版本，`version`                            | `IP` 版本(`4`)和头部长度(`5`)，十六进制 `0x45` 等于十进制 `69`，表示 `IPv4` |
| `0`             | 首部长度，`Internet Header Length`         | 表示 `ip` 首部的长度                                                        |
| `0`             | 服务类型，`Type of Service`                | `0` 表示常规服务                                                            |
| `84`            | 总长度，`Total Length`                     | 整个 `IP` 包的长度，包括 `IP` 头部和数据部分                                |
| `127 172`       | 标识，`Identification`                     | 用于分片重组，和 `Flags`、`Fragment Offset` 联合使用                        |
| `0`             | 标志，`Flags`                              | `0` 保留，`1` 禁止分片，`2` 使用分片                                        |
| `0`             | 片偏移，`Fragment Offset`                  | 分片偏移                                                                    |
| `63`            | 生存时间，`Time to Live`                   | `TTL`，数据包在网络中的生存时间，每经过一个路由器减 `1`                     |
| `1`             | 协议，`Protocol`                           | `1: ICMP`，`2: IGMP`，`6: TCP`，`17: UDP`，`88: IGRP`，`89: OSPF`           |
| `200 126`       | 头部校验和，`Header Checksum`              | 用于检测 `IP` 头部的错误，`IP` 头部校验和为 `0` 表示没有错误                |
| `153 3 238 102` | 起源的 `IP` 地址，`Source IP Address`      | 主机 `ip`                                                                   |
| `172 18 0 2`    | 目的的 `IP` 地址，`Destination IP Address` | 目标 `ip`                                                                   |

`ICMP` 头部：

| 值        | 说明   | 含义                                                             |
| :-------- | :----- | :--------------------------------------------------------------- |
| `0`       | 类型   | `0: ping 应答`，`3: 目的地不可达`，`8: ping 请求`                |
| `0`       | 代码   | `0: 网络不可达`                                                  |
| `255 253` | 校验和 | 用于检测 `ICMP` 头部的错误，`ICMP` 头部校验和为 `0` 表示没有错误 |
| `0 1`     | 标识   | `ICMP` 标识                                                      |
| `0 0`     | 序号   | `ICMP` 序号                                                      |

剩余部分是数据和 `padding`

## mdev 计算

`mdev` 是平均偏差，表示往返时间（`RTT, Round-Trip Time`）的均方误差，`mdev` 较低的值表示网络连接较为稳定，而较高的值则表示延迟波动较大

计算公式：

```
mdev = √(Σ(RTTi - avgRTT)² / n)
```

- `RTTi` 是第 `i` 次的往返时间
- `avgRTT` 是所有往返时间的平均值
- `n` 是往返时间的次数
- `Σ` 是求和
- `√` 是开方根

代码如下：

```go
func mdev() {
  AvgTime = SumTime / float64(i)
  var sum float64 = 0
  for _, time := range times {
    sum += math.Pow(time-AvgTime, 2)
  }
  Mdev = math.Sqrt(sum / float64(i))
}
```