package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"os/signal"
	"ping/utils"
	"time"
)

var (
	typ  uint8 = 8
	code uint8 = 0

	helpFlag bool
	timeout  int64     // 耗时
	interval int64     // 间隔
	size     int       // 大小
	i        int   = 1 // 循环次数

	SendCount int       = 0             // 发送次数
	RecvCount int       = 0             // 接收次数
	MaxTime   float64   = math.MinInt64 // 最大耗时
	MinTime   float64   = math.MaxInt64 // 最短耗时
	SumTime   float64   = 0             // 总计耗时
	AvgTime   float64   = 0
	Mdev      float64   = 0
	times     []float64 = make([]float64, i) // 记录每个请求耗时
)

type Statistics struct {
	startTime time.Time
	since     float64
	cname     string
}

// ICMP 序号不能乱
type ICMP struct {
	Type        uint8  // 类型
	Code        uint8  // 代码
	CheckSum    uint16 // 校验和
	ID          uint16 // ID
	SequenceNum uint16 // 序号
}

var statistics = &Statistics{}

func main() {
	log.SetFlags(log.Llongfile)
	// 解析命令行参数
	parseArgs()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		<-c // 阻塞直到收到信号
		statistics.since = float64(time.Since(statistics.startTime).Nanoseconds())
		total()
		os.Exit(0)
	}()

	// 打印帮助信息
	if helpFlag {
		help()
		os.Exit(0)
	}

	// 获取目标 IP
	domain := os.Args[len(os.Args)-1]
	cname, ips, err := utils.MiekgResolveDomain(domain)
	statistics.cname = cname
	if err != nil {
		log.Println("domain name resolution failed: ", err)
		return
	}

	ip := ips[0].String()
	if ip == "" {
		ip = domain
	}

	// 构建连接
	conn, err := net.DialTimeout("ip:icmp", ip, time.Duration(timeout)*time.Millisecond)

	if err != nil {
		log.Println(err.Error())
		return
	}
	defer conn.Close()

	fmt.Printf("PING %s (%s) %d(%d) bytes of data.\n", cname, ip, size, size+8+20)
	statistics.startTime = time.Now()

	ticker := time.NewTicker(time.Duration(interval) * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		sendICMP(conn, i, size)
		i++
	}
}

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

// func checkSum(data []byte) uint16 {
// 	var sum uint32
// 	for i := 0; i < len(data)-1; i += 2 {
// 		sum += uint32(data[i])<<8 | uint32(data[i+1])
// 	}
// 	if len(data)%2 == 1 {
// 		sum += uint32(data[len(data)-1]) << 8
// 	}
// 	for (sum >> 16) > 0 {
// 		sum = (sum >> 16) + (sum & 0xFFFF)
// 	}
// 	return uint16(^sum)
// }

func sendICMP(conn net.Conn, seq int, size int) error {
	// 构建请求
	icmp := &ICMP{
		Type:        typ,
		Code:        code,
		CheckSum:    uint16(0),
		ID:          uint16(seq),
		SequenceNum: uint16(seq),
	}

	// 将请求转为二进制流
	var buffer bytes.Buffer
	binary.Write(&buffer, binary.BigEndian, icmp)
	data := make([]byte, size)
	buffer.Write(data)
	data = buffer.Bytes()

	checkSum := checkSum(data)
	data[0] = byte(8)
	data[2] = byte(checkSum >> 8)
	data[3] = byte(checkSum)

	startTime := time.Now()

	conn.SetDeadline(time.Now().Add(time.Duration(timeout) * time.Millisecond))

	_, err := conn.Write(data)
	if err != nil {
		return err
	}
	SendCount++

	// IPv4头部通常为20字节
	// ICMP头部为8字节
	// padding 为20字节
	buf := make([]byte, size+20+8+20)
	_, err = conn.Read(buf)
	if err != nil {
		return err
	}
	RecvCount++

	t := float64(time.Since(startTime).Nanoseconds()) / 1e6
	ip := fmt.Sprintf("%d.%d.%d.%d", buf[12], buf[13], buf[14], buf[15])
	fmt.Printf("%d bytes from %s: icmp_seq=%d time=%fms ttl=%d\n", len(data), ip, RecvCount, t, buf[8])
	MaxTime = math.Max(MaxTime, t)
	MinTime = math.Min(MinTime, t)
	SumTime += t
	times = append(times, t)
	return nil
}

func total() {
	mdev()
	t := float64(time.Since(statistics.startTime).Nanoseconds()) / 1e6
	fmt.Printf("\n--- %s ping statistics ---\n", statistics.cname)
	fmt.Printf("%d packets transmitted, %d received, %d packet loss, time %fms\n", SendCount, RecvCount, (i-1)*2-SendCount-RecvCount, t)
	fmt.Printf("rtt min/avg/max/mdev = %f/%f/%f/%f ms\n", MinTime, SumTime/float64(i), MaxTime, Mdev)
}

func mdev() {
	AvgTime = SumTime / float64(i)
	var sum float64 = 0
	for _, time := range times {
		sum += math.Pow(time-AvgTime, 2)
	}
	Mdev = math.Sqrt(sum / float64(i))
}

func help() {
	fmt.Println(`选项：
	-l size        发送缓冲区大小
	-i internal    请求间隔时间(毫秒)
	-w timeout     等待每次回复的超时时间(毫秒)
	-h            帮助选项`)
}

// parseArgs 命令行参数
func parseArgs() {
	flag.Int64Var(&timeout, "w", 1000, "请求超时时间")
	flag.Int64Var(&interval, "i", 1000, "请求间隔时间")
	flag.IntVar(&size, "l", 56, "发送字节数")
	flag.BoolVar(&helpFlag, "h", false, "显示帮助信息")
	flag.Parse()
}
