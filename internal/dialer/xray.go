package dialer

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/GALIAIS/NodeHarvest/internal/model"
)

// BuildXrayOutbound converts protocols supported by xray-core into one outbound.
func BuildXrayOutbound(n *model.Node) (map[string]any, error) {
	if n == nil || !n.IsValid() {
		return nil, fmt.Errorf("invalid node")
	}
	var protocol string
	var settings map[string]any
	switch n.Protocol {
	case model.ProtoSS:
		if n.Password == "" {
			return nil, fmt.Errorf("ss missing password")
		}
		protocol = "shadowsocks"
		settings = map[string]any{"servers": []any{map[string]any{
			"address": n.Server, "port": n.Port, "method": firstNonEmpty(n.Method, "aes-256-gcm"),
			"password": n.Password,
		}}}
	case model.ProtoTrojan:
		if n.Password == "" {
			return nil, fmt.Errorf("trojan missing password")
		}
		protocol = "trojan"
		settings = map[string]any{"servers": []any{map[string]any{
			"address": n.Server, "port": n.Port, "password": n.Password,
		}}}
	case model.ProtoVMess:
		if n.UUID == "" {
			return nil, fmt.Errorf("vmess missing uuid")
		}
		alterID, _ := strconv.Atoi(n.Extra["aid"])
		protocol = "vmess"
		settings = map[string]any{"vnext": []any{map[string]any{
			"address": n.Server, "port": n.Port, "users": []any{map[string]any{
				"id": n.UUID, "alterId": alterID, "security": firstNonEmpty(n.Method, "auto"),
			}},
		}}}
	case model.ProtoVLESS:
		if n.UUID == "" {
			return nil, fmt.Errorf("vless missing uuid")
		}
		user := map[string]any{"id": n.UUID, "encryption": "none"}
		if n.Flow != "" {
			user["flow"] = n.Flow
		}
		protocol = "vless"
		settings = map[string]any{"vnext": []any{map[string]any{
			"address": n.Server, "port": n.Port, "users": []any{user},
		}}}
	default:
		return nil, fmt.Errorf("xray does not support protocol %s", n.Protocol)
	}
	outbound := map[string]any{"tag": "proxy", "protocol": protocol, "settings": settings}
	if stream := buildXrayStream(n); len(stream) > 0 {
		outbound["streamSettings"] = stream
	}
	return outbound, nil
}

func buildXrayStream(n *model.Node) map[string]any {
	network := strings.ToLower(firstNonEmpty(n.Network, "tcp"))
	if network == "websocket" {
		network = "ws"
	}
	stream := map[string]any{"network": network}
	switch network {
	case "ws":
		headers := map[string]any{}
		if n.Host != "" {
			headers["Host"] = n.Host
		}
		stream["wsSettings"] = map[string]any{"path": firstNonEmpty(n.Path, "/"), "headers": headers}
	case "grpc":
		service := firstNonEmpty(n.Extra["serviceName"], strings.TrimPrefix(n.Path, "/"), "GunService")
		stream["grpcSettings"] = map[string]any{"serviceName": service}
	case "http", "h2":
		stream["network"] = "h2"
		stream["httpSettings"] = map[string]any{
			"host": nonEmptySlice(n.Host), "path": firstNonEmpty(n.Path, "/"),
		}
	}
	security := strings.ToLower(n.Security)
	if security == "reality" || n.Extra["pbk"] != "" {
		stream["security"] = "reality"
		stream["realitySettings"] = map[string]any{
			"serverName":  firstNonEmpty(n.SNI, n.Host, n.Server),
			"fingerprint": firstNonEmpty(n.Extra["fp"], "chrome"),
			"publicKey":   n.Extra["pbk"], "shortId": n.Extra["sid"],
		}
	} else if n.TLS || security == "tls" || n.Protocol == model.ProtoTrojan {
		stream["security"] = "tls"
		tlsSettings := map[string]any{
			"serverName":  firstNonEmpty(n.SNI, n.Host, n.Server),
			"fingerprint": firstNonEmpty(n.Extra["fp"], "chrome"),
		}
		if n.SkipTLSVerify() {
			tlsSettings["allowInsecure"] = true
		}
		if n.ALPN != "" {
			tlsSettings["alpn"] = splitCSV(n.ALPN)
		}
		stream["tlsSettings"] = tlsSettings
	}
	return stream
}
