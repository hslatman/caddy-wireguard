module github.com/hslatman/caddy-wireguard

go 1.16

replace github.com/marten-seemann/qtls-go1-15 => github.com/marten-seemann/qtls-go1-16 v0.1.0-beta.1.1

require (
	github.com/caddyserver/caddy/v2 v2.3.0
	go.uber.org/zap v1.16.0
	golang.zx2c4.com/wireguard v0.0.20201119-0.20210113153340-675955de5d0a
)
