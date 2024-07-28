package utils

import (
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

func MiekgResolveDomain(domain string) (string, []net.IP, error) {
	c := new(dns.Client)
	c.Timeout = 5 * time.Second

	m := new(dns.Msg)
	// dns.Fqdn(domain): 这个函数将域名转换为完全限定域名（Fully Qualified Domain Name, FQDN）格式。例如，如果 domain 是 "www.example.com"，dns.Fqdn(domain) 会返回 "www.example.com."（注意末尾的点）。这是 DNS 查询所需的标准格式。
	// dns.TypeA: 这指定了我们要查询的 DNS 记录类型。TypeA 表示我们要查询 IPv4 地址记录。如果我们想查询 IPv6 地址，可以使用 dns.TypeAAAA。
	m.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	// 223.5.5.5:53 是阿里的 DNS 服务器
	// 8.8.8.8:53 是 Google 的 DNS 服务器
	r, _, err := c.Exchange(m, "223.5.5.5:53") // 使用 Google 的 DNS 服务器
	if err != nil {
		return "", nil, err
	}

	var cname string
	var ips []net.IP

	for _, ans := range r.Answer {
		switch record := ans.(type) {
		case *dns.CNAME:
			cname = strings.TrimSuffix(record.Target, ".")
		case *dns.A:
			ips = append(ips, record.A)
		}
	}

	if cname == "" {
		cname = domain
	}

	return cname, ips, nil
}
