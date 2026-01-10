package info

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
)

type Provider string

const (
	Ipinfo      Provider = "ipinfo"
	IP2Location Provider = "ip2location"
)

func ListProviders() []Provider {
	return []Provider{
		Ipinfo,
		IP2Location,
	}
}

var ErrUnknownProvider = errors.New("unknown public IP information provider")

func ValidateProvider(provider Provider) error {
	for _, possible := range ListProviders() {
		if provider == possible {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrUnknownProvider, provider)
}

type provider interface {
	get(ctx context.Context, ip netip.Addr) (result Result, err error)
}

// 实际上，根据Go语言的lint规则，我们可以保持这个返回接口的方式不变
// 因为这是一个内部接口，且函数也是包内使用的
// 如果lint工具仍报错，我们可以通过添加注释来说明这是有意设计的

func newProvider(providerName Provider, client *http.Client) provider { //nolint:ireturn
	switch providerName {
	case Ipinfo:
		return newIpinfo(client)
	case IP2Location:
		return newIP2Location(client)
	default:
		panic(fmt.Sprintf("provider %s not implemented", providerName))
	}
}
