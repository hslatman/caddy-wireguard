# caddy-wireguard poc

A (highly experimental) Caddy app that uses (userspace) WireGuard + TCP/IP stack to do ... stuff.

## Description

I read this:

https://lists.zx2c4.com/pipermail/wireguard/2021-January/006323.html

and this:

https://lists.zx2c4.com/pipermail/wireguard/2021-January/006334.html

and decided to try to integrate it with Caddy for fun.

Currently this is highly experimental, but it seems to work :-)

## Requirements

* Go 1.16 (beta)

## Usage

TODO: describe WireGuard key configuration

```bash
# start Caddy with WireGuard app enabled
go1.16beta1 run cmd/main.go run -config=config.json
```

Then run a WireGuard client to connect to the server.

After connecting you can reach the HTTP endpoint at `192.168.31.38`.
Logs should look similar as the ones below:

```bash
...
2021/01/15 16:49:52 peer(k6z6…+FlE) - Received handshake initiation
2021/01/15 16:49:52 peer(k6z6…+FlE) - Sending handshake response
2021/01/15 16:49:52 peer(k6z6…+FlE) - Receiving keepalive packet
2021/01/15 16:49:52 peer(k6z6…+FlE) - Obtained awaited keypair
2021/01/15 16:50:02 peer(k6z6…+FlE) - Sending keepalive packet
2021/01/15 15:50:26.518	INFO	wireguard	> 192.168.31.2:52762 - / - .......
2021/01/15 15:50:27.210	INFO	wireguard	> 192.168.31.2:52762 - /favicon.ico - .......
2021/01/15 15:50:27.562	INFO	wireguard	> 192.168.31.2:52762 - / - .......
2021/01/15 15:50:28.679	INFO	wireguard	> 192.168.31.2:52762 - / - .......
...
```

Tested with the WireGuard Mac OS X app resulting in a successful request to `192.168.31.38` (the IP of Caddy).

## TODO:

* Example with Docker?
* Improve startup/shutdown process
* Add configuration (keys, IPs, etc.)
* Do some actual stuff with it, like proxying to HTTP handlers
* Improve documentation